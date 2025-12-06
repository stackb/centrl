package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	slpb "github.com/stackb/centrl/build/stack/starlark/v1beta1"
	"github.com/stackb/centrl/pkg/stardoc"
)

func extractDocumentationInfo(cfg *config, bzlFileByPath map[string]*bzlFile, filesToExtract []string) (*bzpb.DocumentationInfo, error) {
	result := &bzpb.DocumentationInfo{
		Source: bzpb.DocumentationSource_BEST_EFFORT,
	}

	var errors int
	for _, filePath := range filesToExtract {
		bzlFile, found := bzlFileByPath[filePath]
		if !found {
			return nil, fmt.Errorf("no file %q was found in the list of --bzl_file", filePath)
		}

		module, err := extractModule(cfg, bzlFile)
		if err != nil {
			file := &bzpb.FileInfo{Label: bzlFile.Label, Error: err.Error()}
			result.File = append(result.File, file)
			cfg.Logger.Printf("🔴 failed to extract module info for %v: %v", bzlFile.EffectivePath, err)
			errors++
		} else {
			file := stardoc.ModuleToFileInfo(module)
			file.Label = bzlFile.Label
			result.File = append(result.File, file)
			cfg.Logger.Println("🟢", bzlFile.EffectivePath)
		}
	}

	// Report success rate
	total := len(cfg.FilesToExtract)
	success := total - errors
	pct := float64(success) / float64(total) * 100.0
	cfg.Logger.Printf("Extraction: %d/%d %.1f%%", success, total, pct)

	return result, nil
}

func extractModule(cfg *config, file *bzlFile) (*slpb.Module, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	response, err := cfg.Client.ModuleInfo(ctx, &slpb.ModuleInfoRequest{
		TargetFileLabel: file.EffectivePath,
		BuiltinsBzlPath: filepath.Join(cfg.Cwd, workDir, "external/_builtins/src/main/starlark/builtins_bzl"),
		DepRoots: []string{
			filepath.Join(cfg.Cwd, workDir),
		},
	})
	if err != nil {
		// Strip absolute path prefix from error messages
		cleanErr := cleanErrorMessage(err, cfg.Cwd)
		return nil, fmt.Errorf("ModuleInfo request error: %v", cleanErr)
	}

	return response, nil
}
