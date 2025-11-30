package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/buildtools/build"
	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	slpb "github.com/stackb/centrl/build/stack/starlark/v1beta1"

	"github.com/stackb/centrl/pkg/stardoc"
)

type bzlFile struct {
	RepoName      string // the name of the repo to which the file belongs (e.g. "rules_go")
	Path          string // the original path
	EffectivePath string // the remapped path
	Label         *bzpb.Label
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
type bzlFileSlice []*bzlFile

func (s *bzlFileSlice) String() string {
	var parts []string
	for _, sf := range *s {
		parts = append(parts, fmt.Sprintf("%s:%s", sf.RepoName, sf.Path))
	}
	return strings.Join(parts, ",")
}

func (s *bzlFileSlice) Set(value string) error {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid mapping format %q, expected REPO_NAME=PATH", value)
	}

	*s = append(*s, &bzlFile{
		RepoName: parts[0],
		Path:     parts[1],
	})

	return nil
}

// moduleDepsMap is a custom flag type for repeatable --module_dep flags
type moduleDepsMap map[string][]*bzpb.ModuleDependency

func (m *moduleDepsMap) String() string {
	if *m == nil {
		return ""
	}
	var parts []string
	for docsRepo, deps := range *m {
		parts = append(parts, fmt.Sprintf("%s=%+v", docsRepo, deps))
	}
	return strings.Join(parts, ",")
}

func (m *moduleDepsMap) Set(value string) error {
	if *m == nil {
		*m = make(map[string][]*bzpb.ModuleDependency)
	}

	// Parse DOCS_REPO_NAME=MODULE_NAME:MODULE_VERSION:REPO_NAME
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid module_dep format %q, expected DOCS_REPO_NAME=MODULE_NAME:MODULE_VERSION:REPO_NAME", value)
	}

	docsRepoName := parts[0]
	depParts := strings.Split(parts[1], "=")
	if len(depParts) != 2 {
		return fmt.Errorf("invalid module_dep format %q, expected MODULE_NAME:DEP_NAME=REPO_NAME after =", value)
	}

	(*m)[docsRepoName] = append((*m)[docsRepoName], &bzpb.ModuleDependency{
		Name:     depParts[0],
		RepoName: depParts[1],
	})

	return nil
}

func extractDocumentation(cfg *Config) (*bzpb.DocumentationInfo, error) {

	bzlFileByPath := make(map[string]*bzlFile)

	result := &bzpb.DocumentationInfo{
		Source: bzpb.DocumentationSource_BEST_EFFORT,
	}

	for _, file := range cfg.BzlFiles {
		bzlFileByPath[file.Path] = file
		if err := rewriteFile(cfg, file); err != nil {
			return nil, err
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

	if debugSandbox {
		listFiles(cfg.Logger, "external")
	}

	var errors []error

	for _, filePath := range cfg.FilesToExtract {
		bzlFile, found := bzlFileByPath[filePath]
		if !found {
			return nil, fmt.Errorf("no file %q was found in the list of --bzl_file", filePath)
		}
		module, err := extractModule(cfg, bzlFile)
		if err != nil {
			errors = append(errors, fmt.Errorf("🔴 %v\n%v", bzlFile.EffectivePath, err))
			result.File = append(result.File, &bzpb.FileInfo{
				Label:       bzlFile.Label,
				Description: "Autogenerated documentation",
				Error:       err.Error(),
			})
			// logger.Printf("🔴 failed to extract module info for %v: %v", src, err)
		} else {
			file := stardoc.ParseModuleFile(module.Info)
			result.File = append(result.File, file)
			cfg.Logger.Println("🟢", bzlFile.EffectivePath)
		}
	}

	// Report success rate
	total := len(cfg.FilesToExtract)
	success := total - len(errors)
	pct := float64(success) / float64(total) * 100.0
	cfg.Logger.Printf("Extraction: %d/%d %.1f%%", success, total, pct)

	if len(errors) > 0 {
		for _, err := range errors {
			cfg.Logger.Println(err.Error())
		}
	}

	cfg.Logger.Printf("Extraction: %d/%d %.1f%%", success, total, pct)

	return result, nil
}

func extractModule(cfg *Config, file *bzlFile) (*slpb.Module, error) {

	var content string

	// logger.Printf("extracting module for: %v", file)
	request := &slpb.ModuleInfoRequest{
		TargetFileLabel:     file.EffectivePath,
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

// listFiles is a convenience debugging function to log the files under a given dir.
func listFiles(logger *log.Logger, dir string) error {
	logger.Println("Listing files under " + dir)
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Printf("%v\n", err)
			return err
		}
		if !strings.HasPrefix(path, "external/rules_java") {
			logger.Println(path)
		}
		return nil
	})
}

func rewriteFile(cfg *Config, file *bzlFile) error {
	parts := strings.Split(file.Path, "/")
	if parts[0] != "external" {
		return fmt.Errorf("--bzl_file expected to be in an external workspace: %s", file.Path)
	}
	rest := filepath.Join(parts[2:]...)

	file.Label = &bzpb.Label{
		Repo: file.RepoName,
		Pkg:  filepath.Dir(rest),
		Name: filepath.Base(rest),
	}

	deps, found := cfg.moduleDeps[file.RepoName]
	if !found {
		cfg.Logger.Printf("WARN: dependencies for %s not found", file.RepoName)
	}

	srcPath := filepath.Join(cfg.Cwd, file.Path)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("rewriteFile: src not found: %s", srcPath)
	}

	ast, loads, data, err := readBzlFile(cfg, srcPath)
	if err != nil {
		return err
	}

	for _, load := range loads {
		from, err := label.Parse(load.Module.Value)
		if err != nil {
			return fmt.Errorf("failed to parse label %s in module %s: %v", load.Module.Value, file.Path, err)
		}
		to := from.Abs(file.RepoName, file.Label.Pkg)

		if from.Repo == "" {
			to.Repo = file.RepoName
		} else if from.Repo == file.RepoName {
			to.Repo = file.RepoName
		} else if from.Repo == "bazel_tools" {
			to.Repo = "bazel_tools"
		} else {
			var match *bzpb.ModuleDependency
			for _, dep := range deps {
				if dep.RepoName == from.Repo {
					to.Repo = dep.Name
					match = dep
				} else if dep.Name == from.Repo {
					to.Repo = dep.Name
					match = dep
				}
			}
			if match == nil {
				cfg.Logger.Printf("WARN: unknown dependency @%s of module %s (%s)", from.Repo, file.RepoName, file.Path)
			}
		}
		if from != to {
			load.Module.Value = to.String()
			cfg.Logger.Printf("rewrote load: %s --> %s (%s)", from, to, file.Path)
		}
	}

	if ast != nil {
		data = build.Format(ast)
	}

	file.EffectivePath = filepath.Join("external", file.RepoName, rest)
	dstPath := filepath.Join(cfg.Cwd, file.EffectivePath)

	return writeFile(dstPath, data, os.ModePerm)
}

func writeFile(dst string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func readBzlFile(cfg *Config, path string) (*build.File, []*build.LoadStmt, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("os.ReadFile(%q) error: %v", path, err)
	}
	ast, err := build.ParseBzl(path, data)
	if err != nil {
		if failOnParseErrors {
			return nil, nil, nil, fmt.Errorf("build.Parse(%q) error: %v", data, err)
		} else {
			cfg.Logger.Printf("WARN: build.Parse(%q) error: %v", data, err)
		}
	}
	if ast == nil {
		return ast, nil, data, nil
	}

	var loads []*build.LoadStmt
	build.WalkOnce(ast, func(expr *build.Expr) {
		n := *expr
		if l, ok := n.(*build.LoadStmt); ok {
			loads = append(loads, l)
		}
	})

	return ast, loads, data, nil
}
