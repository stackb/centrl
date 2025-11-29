package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	slpb "github.com/stackb/centrl/build/stack/starlark/v1beta1"

	"github.com/stackb/centrl/pkg/paramsfile"
	"github.com/stackb/centrl/pkg/stardoc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	wppb "github.com/stackb/centrl/blaze/worker"
	"github.com/stackb/centrl/pkg/protoutil"
)

const (
	debugArgs    = true
	debugSandbox = true
	failOnErrors = true
)
const toolName = "starlarkcompiler"

type Config struct {
	BuiltinsBzlPath      string // TODO: load bb //src/main/starlark/builtins_bzl:builtins_bzl_zip?
	Client               slpb.StarlarkClient
	Cwd                  string
	JavaInterpreterFile  string
	LogFile              string
	Logger               *log.Logger
	OutputFile           string
	PersistentWorker     bool
	Port                 int
	ServerJarFile        string
	ModuleDepsByRepoName moduleDepsMap // moddule dependencies for a given bzl_srcs repo_name
	BzlFiles             bzlFileSlice  // the transitive set of .bzl files in the sandbox
	FilesToExtract       []string      // list of files to extract docs for
	WorkspaceCwd         string
	WorkspaceOutputBase  string
	BazelToolsRepoName   string
}

type BzlFile struct {
	RepoName string
	Path     string
}

func (file *BzlFile) Label() *bzpb.Label {
	var label bzpb.Label
	label.Repo = file.RepoName
	label.Pkg = labelPkg(file.RepoName, file.Path)
	label.Name = path.Base(file.Path)
	return &label
}

// labelPkg extracts the package path from a full path given a repo name.
// For example, given repoName "rules_go_0.51.0_docs" and
// path "external/build_stack_rules_proto++starlark_repository+rules_go_0.51.0_docs/docs/doc_helpers.bzl",
// it returns "docs".
func labelPkg(repoName, fullPath string) string {
	// Remove "external/" prefix if present
	pathToSearch := strings.TrimPrefix(fullPath, "external/")

	// Find the first path component that ends with the repo name
	parts := strings.Split(pathToSearch, "/")

	for i, part := range parts {
		if strings.HasSuffix(part, repoName) {
			// Found the repo component, return everything after it except the filename
			if i+1 < len(parts)-1 {
				return filepath.Join(parts[i+1 : len(parts)-1]...)
			}
			return ""
		}
	}

	// If we didn't find the repo name, return the directory of the path
	return filepath.Dir(pathToSearch)
}

// bzlFileSlice is a custom flag type for repeatable --mapping flags
type bzlFileSlice []BzlFile

func (s *bzlFileSlice) String() string {
	var parts []string
	for _, sf := range *s {
		parts = append(parts, fmt.Sprintf("%s=%s", sf.RepoName, sf.Path))
	}
	return strings.Join(parts, ",")
}

func (s *bzlFileSlice) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid mapping format %q, expected LABEL=PATH", value)
	}

	*s = append(*s, BzlFile{
		RepoName: parts[0],
		Path:     parts[1],
	})

	return nil
}

// moduleDepsMap is a custom flag type for repeatable --module_dep flags
type moduleDepsMap map[string]*bzpb.ModuleDependency

func (m *moduleDepsMap) String() string {
	if *m == nil {
		return ""
	}
	var parts []string
	for docsRepo, dep := range *m {
		parts = append(parts, fmt.Sprintf("%s=%s:%s:%s", docsRepo, dep.Name, dep.Version, dep.RepoName))
	}
	return strings.Join(parts, ",")
}

func (m *moduleDepsMap) Set(value string) error {
	if *m == nil {
		*m = make(map[string]*bzpb.ModuleDependency)
	}

	// Parse DOCS_REPO_NAME=MODULE_NAME:MODULE_VERSION:REPO_NAME
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid module_dep format %q, expected DOCS_REPO_NAME=MODULE_NAME:MODULE_VERSION:REPO_NAME", value)
	}

	docsRepoName := parts[0]
	depParts := strings.Split(parts[1], ":")
	if len(depParts) != 3 {
		return fmt.Errorf("invalid module_dep format %q, expected MODULE_NAME:MODULE_VERSION:REPO_NAME after =", value)
	}

	(*m)[docsRepoName] = &bzpb.ModuleDependency{
		Name:     depParts[0],
		RepoName: depParts[1],
		Version:  depParts[2],
	}

	return nil
}

func main() {
	log.SetPrefix(toolName + ": ")
	log.SetOutput(os.Stderr)
	log.SetFlags(0) // don't print timestamps

	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {

	parsedArgs, err := paramsfile.ReadArgsParamsFile(args)

	if debugArgs {
		log.Println("args:", args)
		for _, arg := range parsedArgs {
			log.Println("parsedArg:", arg)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to read params file: %v", err)
	}

	cfg, err := parseFlags(parsedArgs)
	if err != nil {
		return fmt.Errorf("failed to parse args: %v", err)
	}

	// Validate required flags
	if cfg.JavaInterpreterFile == "" {
		return fmt.Errorf("--java_interpreter_file is required")
	}
	if cfg.ServerJarFile == "" {
		return fmt.Errorf("--server_jar_file is required")
	}
	if cfg.OutputFile == "" {
		return fmt.Errorf("--output_file is required")
	}
	if cfg.WorkspaceCwd == "" {
		return fmt.Errorf("--workspace_cwd is required")
	}
	if cfg.WorkspaceOutputBase == "" {
		return fmt.Errorf("--workspace_output_base is required")
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting os cwd: %v", err)
	} else {
		cfg.Cwd = wd
	}

	// Initialize logger
	if cfg.LogFile != "" {
		logFile, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("failed to open log file: %v", err)
		}
		defer logFile.Close()
		cfg.Logger = log.New(logFile, toolName+": ", log.LstdFlags)
	} else {
		cfg.Logger = log.New(os.Stderr, toolName+": ", 0)
	}

	// Initialize gRPC client
	if cfg.Port == 0 {
		cfg.Port = mustGetFreePort()
	}
	conn, err := grpc.NewClient(
		fmt.Sprintf("localhost:%d", cfg.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to create gRPC client: %v", err)
	}
	defer conn.Close()

	cfg.Client = slpb.NewStarlarkClient(conn)

	if cfg.PersistentWorker {
		if err := persistentWork(&cfg); err != nil {
			return fmt.Errorf("while performing persistent work: %v", err)
		}
	} else {
		if err := batchWork(&cfg); err != nil {
			return fmt.Errorf("while performing batch work: %v", err)
		}
	}

	return nil
}

func persistentWork(cfg *Config) error {
	for {
		var req wppb.WorkRequest
		if err := protoutil.ReadDelimitedFrom(&req, os.Stdin); err != nil {
			if err == io.EOF {
				// this is the signal to terminate the program
				break
			}
			return fmt.Errorf("reading work request: %v", err)
		}

		var resp wppb.WorkResponse
		resp.RequestId = req.RequestId

		batchCfg, err := parseFlags(req.Arguments)
		if err != nil {
			return fmt.Errorf("parsing work request arguments: %v", err)
		}

		batchCfg.Client = cfg.Client
		batchCfg.Cwd = cfg.Cwd
		batchCfg.Logger = cfg.Logger

		if err := batchWork(&batchCfg); err != nil {
			return fmt.Errorf("performing persistent batch!: %v", err)
		}

		if err := protoutil.WriteDelimitedTo(&resp, os.Stdout); err != nil {
			return fmt.Errorf("writing work response: %v", err)
		}
	}
	return nil
}

func batchWork(cfg *Config) error {
	if cfg.OutputFile == "" {
		return fmt.Errorf("--output_file is required")
	}

	now := time.Now()
	fail := func(err error) error {
		return fmt.Errorf("%v (%v)", err, time.Since(now))
	}

	cfg.Logger.Printf("Processing %d source files", len(cfg.BzlFiles))

	result, err := extractDocumentation(cfg)
	if err != nil {
		return fail(fmt.Errorf("failed to extract module info: %v", err))
	}

	if err := protoutil.WriteFile(cfg.OutputFile, result); err != nil {
		return fail(fmt.Errorf("failed to write output file: %v", err))
	}

	cfg.Logger.Printf("Completed in %v", time.Since(now))

	return nil
}

func parseFlags(args []string) (cfg Config, err error) {
	fs := flag.NewFlagSet(toolName, flag.ExitOnError)
	var moduleDepsByRepoName moduleDepsMap

	fs.StringVar(&cfg.JavaInterpreterFile, "java_interpreter_file", "", "path to a java interpreter")
	fs.StringVar(&cfg.ServerJarFile, "server_jar_file", "", "the executable jar file for the server")
	fs.StringVar(&cfg.LogFile, "log_file", "", "path to log file (optional, defaults to stderr)")
	fs.StringVar(&cfg.OutputFile, "output_file", "", "the output file to write")
	fs.StringVar(&cfg.WorkspaceCwd, "workspace_cwd", "", "the workspace root dir")
	fs.StringVar(&cfg.WorkspaceOutputBase, "workspace_output_base", "", "workspace output base")
	fs.StringVar(&cfg.BazelToolsRepoName, "bazel_tools_repo_name", "", "the canonical repository name for bazel tools sources")
	fs.IntVar(&cfg.Port, "port", 0, "the port number to use for the server process")
	fs.BoolVar(&cfg.PersistentWorker, "persistent_worker", false, "present if this tool is being invokes as a bazel persistent worker")
	fs.Var(&cfg.BzlFiles, "bzl_file", "bzl source file mapping in the format LABEL=PATH (repeatable)")
	fs.Var(&moduleDepsByRepoName, "module_dep", "module dependency mapping in the format DOCS_REPO_NAME=MODULE_NAME:REPO_NAME:MODULE_VERSION (repeatable)")
	fs.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s @PARAMS_FILE", toolName)
		fs.PrintDefaults()
	}

	if err = fs.Parse(args); err != nil {
		return
	}

	cfg.FilesToExtract = fs.Args()
	cfg.ModuleDepsByRepoName = moduleDepsByRepoName

	if len(cfg.BzlFiles) == 0 {
		return cfg, fmt.Errorf("--bzl_file list must not be empty")
	}
	if len(cfg.FilesToExtract) == 0 {
		return cfg, fmt.Errorf("extract file list must not be empty")
	}

	return
}

func extractDocumentation(cfg *Config) (*bzpb.DocumentationInfo, error) {

	result := &bzpb.DocumentationInfo{
		Source: bzpb.DocumentationSource_BEST_EFFORT,
	}

	files := make(map[string]BzlFile)
	for _, file := range cfg.BzlFiles {
		files[file.Path] = file
	}
	var filesToExtract []BzlFile
	for _, filePath := range cfg.FilesToExtract {
		if file, ok := files[filePath]; ok {
			filesToExtract = append(filesToExtract, file)
		} else {
			return nil, fmt.Errorf("no file %q was found in the list of --bzl_file", filePath)
		}
	}

	for _, file := range cfg.BzlFiles {
		parts := strings.Split(file.Path, "/")
		if parts[0] == "external" {
			repoName := parts[1]
			rest := filepath.Join(parts[2:]...)
			repoNameParts := strings.Split(repoName, "+")
			if repoName == cfg.BazelToolsRepoName {
				workspaceName := "bazel"
				srcPath := filepath.Join(cfg.Cwd, file.Path)
				dstPath := filepath.Join(cfg.Cwd, "external", workspaceName, rest)
				if err := copyFile(srcPath, dstPath, os.ModePerm); err != nil {
					return nil, err
				}
				workspaceName = "bazel_tools"
				srcPath = filepath.Join(cfg.Cwd, file.Path)
				dstPath = filepath.Join(cfg.Cwd, "external", workspaceName, rest)
				if err := copyFile(srcPath, dstPath, os.ModePerm); err != nil {
					return nil, err
				}
			} else {
				workspaceName := repoNameParts[len(repoNameParts)-1]
				srcPath := filepath.Join(cfg.Cwd, file.Path)
				dstPath := filepath.Join(cfg.Cwd, "external", workspaceName, rest)
				if err := copyFile(srcPath, dstPath, os.ModePerm); err != nil {
					return nil, err
				}
			}

			// log.Printf("copied: %s -> %s", srcPath, dstPath)
			// if err := os.MkdirAll(filepath.Dir(dstPath), os.ModePerm); err != nil {
			// 	return nil, fmt.Errorf("setting up dst dir: %v", err)
			// }
			// if err := os.Symlink(srcPath, dstPath); err != nil {
			// 	return nil, fmt.Errorf("setting up dst symlink: %v", err)
			// }
			// log.Printf("symlinked: %s -> %s", srcPath, dstPath)
		} else {
			log.Fatalf("rly? %v", parts)
		}
	}

	if err := writeFile(filepath.Join(cfg.Cwd, "external/host_platform/constraints.bzl"), []byte(`
HOST_CONSTRAINTS = []
`), os.ModePerm); err != nil {
		return nil, err
	}

	if err := writeFile(filepath.Join(cfg.Cwd, "external/bazel_features_version/version.bzl"), []byte("version = '8.4.2'\n"), os.ModePerm); err != nil {
		return nil, err
	}
	if err := writeFile(filepath.Join(cfg.Cwd, "external/bazel_features_globals/globals.bzl"), []byte(`
globals = struct(
    CcSharedLibraryInfo = CcSharedLibraryInfo,
    CcSharedLibraryHintInfo = CcSharedLibraryHintInfo,
    macro = macro,
    PackageSpecificationInfo = PackageSpecificationInfo,
    RunEnvironmentInfo = RunEnvironmentInfo,
    subrule = subrule,
    DefaultInfo = DefaultInfo,
    __TestingOnly_NeverAvailable = None,
    JavaInfo = getattr(getattr(native, 'legacy_globals', None), 'JavaInfo', None),
    JavaPluginInfo = getattr(getattr(native, 'legacy_globals', None), 'JavaPluginInfo', None),
    ProtoInfo = getattr(getattr(native, 'legacy_globals', None), 'ProtoInfo', None),
    PyCcLinkParamsProvider = getattr(getattr(native, 'legacy_globals', None), 'PyCcLinkParamsProvider', None),
    PyInfo = getattr(getattr(native, 'legacy_globals', None), 'PyInfo', None),
    PyRuntimeInfo = getattr(getattr(native, 'legacy_globals', None), 'PyRuntimeInfo', None),
    cc_proto_aspect = getattr(getattr(native, 'legacy_globals', None), 'cc_proto_aspect', None),
)`), os.ModePerm); err != nil {
		return nil, err
	}

	log.Fatalln("STOP")

	if debugSandbox {
		listFiles(".")
	}

	var errors []error
	for _, file := range filesToExtract {
		// if src.From.String() != "@@build_stack_rules_proto++starlark_repository+rules_java_9.0.3_docs//java/bazel/rules:libbazel_java_binary" {
		// 	continue
		// }

		module, err := extractModule(cfg, file)
		if err != nil {
			errors = append(errors, fmt.Errorf("🔴 unable to extract module info for %v: %v", file, err))
			result.File = append(result.File, &bzpb.FileInfo{
				Label:       file.Label(),
				Description: "Autogenerated documentation",
				Error:       err.Error(),
			})
			// log.Printf("🔴 failed to extract module info for %v: %v", src, err)
		} else {
			file := stardoc.ParseModuleFile(module.Info)
			result.File = append(result.File, file)
			log.Println("🟢 extracted module info:", file)
		}
	}

	if failOnErrors && len(errors) > 0 {
		for _, err := range errors {
			log.Println(err.Error())
		}
		log.Fatalln("FAILED")
	}

	return result, nil
}

func extractModule(cfg *Config, file BzlFile) (*slpb.Module, error) {

	var content string

	log.Printf("extracting module for: %v", file)
	request := &slpb.ModuleInfoRequest{
		TargetFileLabel:     file.Path,
		WorkspaceName:       "_main",
		WorkspaceCwd:        cfg.WorkspaceCwd,
		WorkspaceOutputBase: cfg.WorkspaceOutputBase,
		Rel:                 "",
		BuiltinsBzlPath:     cfg.BuiltinsBzlPath,
		ModuleContent:       content,
		DepRoots:            []string{cfg.Cwd},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	response, err := cfg.Client.ModuleInfo(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("ModuleInfo request error: %v", err)
	}

	return response, nil
}

func mustGetFreePort() int {
	port, err := getFreePort()
	if err != nil {
		log.Panicf("Unable to determine free port: %v", err)
	}
	return port
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// listFiles is a convenience debugging function to log the files under a given dir.
func listFiles(dir string) error {
	log.Println("Listing files under " + dir)
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("%v\n", err)
			return err
		}
		if !strings.HasPrefix(path, "external/rules_java") {
			log.Println(path)
		}
		return nil
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("copyFile: src not found: %s", src)
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return writeFile(dst, data, mode)
}

func writeFile(dst string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}
