package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	slpb "github.com/stackb/centrl/build/stack/starlark/v1beta1"

	"github.com/stackb/centrl/pkg/paramsfile"

	wppb "github.com/stackb/centrl/blaze/worker"
	"github.com/stackb/centrl/pkg/protoutil"
)

const (
	debugArgs           = true
	debugSandbox        = false
	failOnParseErrors   = false
	failOnExtractErrors = true
)
const toolName = "starlarkcompiler"

type Config struct {
	BuiltinsBzlPath     string // TODO: load bb //src/main/starlark/builtins_bzl:builtins_bzl_zip?
	Client              slpb.StarlarkClient
	Cwd                 string
	JavaInterpreterFile string
	LogFile             string
	Logger              *log.Logger
	OutputFile          string
	PersistentWorker    bool
	Port                int
	ServerJarFile       string
	BzlFiles            bzlFileSlice // the transitive set of .bzl files in the sandbox
	FilesToExtract      []string     // list of files to extract docs for
	WorkspaceCwd        string
	WorkspaceOutputBase string
	BazelToolsRepoName  string
	moduleDeps          moduleDepsMap
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
	if debugArgs {
		log.Println("args:", args)
	}
	parsedArgs, err := paramsfile.ReadArgsParamsFile(args)
	if err != nil {
		return fmt.Errorf("failed to read params file: %v", err)
	}
	if debugArgs {
		for _, arg := range parsedArgs {
			log.Println("parsedArg:", arg)
		}
	}

	cfg, err := parseConfig(parsedArgs)
	if err != nil {
		return fmt.Errorf("failed to parse args: %v", err)
	}

	if cfg.PersistentWorker {
		if err := runPersistent(cfg); err != nil {
			return fmt.Errorf("while performing persistent work: %v", err)
		}
		cfg.Logger.Println("Received EOF, shutting down persistent worker")
	} else {
		// Initialize server and gRPC client for batch work
		resources, cleanup, err := initializeServer(cfg.JavaInterpreterFile, cfg.ServerJarFile, cfg.Port, cfg.LogFile, cfg.Logger)
		if err != nil {
			return fmt.Errorf("failed to initialize server: %v", err)
		}
		cfg.Logger.Println("Server ready")
		defer cleanup()

		cfg.Client = slpb.NewStarlarkClient(resources.conn)

		if err := batchWork(cfg); err != nil {
			return fmt.Errorf("while performing batch work: %v", err)
		}
	}

	return nil
}

func runPersistent(persistentCfg *Config) error {
	var resources *serverResources

	for {
		var req wppb.WorkRequest
		if err := protoutil.ReadDelimitedFrom(&req, os.Stdin); err != nil {
			if err == io.EOF {
				// this is the signal to terminate the program
				return nil
			}
			return fmt.Errorf("reading work request: %v", err)
		}

		var resp wppb.WorkResponse
		resp.RequestId = req.RequestId

		batchCfg, err := parseConfig(req.Arguments)
		if err != nil {
			// Don't kill the worker on parse errors - report error and continue
			errMsg := fmt.Sprintf("parsing work request arguments: %v", err)
			persistentCfg.Logger.Println("ERROR:", errMsg)
			resp.Output = errMsg
			resp.ExitCode = 1
		} else {
			// Start server on first request
			if resources == nil {
				if batchCfg.Port == 0 {
					batchCfg.Port = mustGetFreePort(persistentCfg.Logger)
				}

				r, cleanup, err := initializeServer(batchCfg.JavaInterpreterFile, batchCfg.ServerJarFile, batchCfg.Port, batchCfg.LogFile, persistentCfg.Logger)
				if err != nil {
					errMsg := fmt.Sprintf("failed to initialize server: %v", err)
					batchCfg.Logger.Println("ERROR:", errMsg)
					resp.Output = errMsg
					resp.ExitCode = 1
					if err := protoutil.WriteDelimitedTo(&resp, os.Stdout); err != nil {
						return fmt.Errorf("writing work response: %v", err)
					}
					os.Stdout.Sync()
					continue
				}
				resources = r
				defer cleanup()
				persistentCfg.Port = batchCfg.Port
				persistentCfg.Client = slpb.NewStarlarkClient(resources.conn)
				persistentCfg.Logger.Println("Server ready")
			}

			batchCfg.Logger = persistentCfg.Logger
			batchCfg.Cwd = persistentCfg.Cwd
			batchCfg.Port = persistentCfg.Port
			batchCfg.Client = persistentCfg.Client

			if err := batchWork(batchCfg); err != nil {
				// Don't kill the worker on work errors - report error and continue
				errMsg := fmt.Sprintf("performing work: %v", err)
				persistentCfg.Logger.Println("ERROR:", errMsg)
				resp.Output = errMsg
				resp.ExitCode = 1
			} else {
				// Success
				resp.ExitCode = 0
			}
		}

		if err := protoutil.WriteDelimitedTo(&resp, os.Stdout); err != nil {
			return fmt.Errorf("writing work response: %v", err)
		}

		// Flush stdout to ensure Bazel receives the response immediately
		os.Stdout.Sync()
	}
}

func batchWork(cfg *Config) error {

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

func parseConfig(args []string) (*Config, error) {
	var cfg Config

	fs := flag.NewFlagSet(toolName, flag.ExitOnError)
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
	fs.Var(&cfg.moduleDeps, "module_dep", "module dependency map (repeatable)")
	fs.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s @PARAMS_FILE", toolName)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting os cwd: %v", err)
	}
	cfg.Cwd = wd

	if cfg.LogFile != "" {
		logFile, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %v", err)
		}
		defer logFile.Close()
		cfg.Logger = log.New(logFile, toolName+": ", log.LstdFlags)
	} else {
		cfg.Logger = log.New(os.Stderr, toolName+": ", 0)
	}

	// For persistent worker mode, we only get --persistent_worker flag initially.
	// All other flags come in WorkRequest messages.
	if cfg.PersistentWorker {
		return &cfg, nil
	}

	cfg.FilesToExtract = fs.Args()

	if cfg.OutputFile == "" {
		return nil, fmt.Errorf("--output_file is required")
	}
	if len(cfg.BzlFiles) == 0 {
		return nil, fmt.Errorf("--bzl_file list must not be empty")
	}
	if len(cfg.FilesToExtract) == 0 {
		return nil, fmt.Errorf("extract file list must not be empty")
	}
	if cfg.JavaInterpreterFile == "" {
		return nil, fmt.Errorf("--java_interpreter_file is required")
	}
	if cfg.ServerJarFile == "" {
		return nil, fmt.Errorf("--server_jar_file is required")
	}
	if cfg.OutputFile == "" {
		return nil, fmt.Errorf("--output_file is required")
	}
	if cfg.WorkspaceCwd == "" {
		return nil, fmt.Errorf("--workspace_cwd is required")
	}
	if cfg.WorkspaceOutputBase == "" {
		return nil, fmt.Errorf("--workspace_output_base is required")
	}

	return &cfg, nil
}
