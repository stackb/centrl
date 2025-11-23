package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/bazelbuild/bazel-gazelle/label"
	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	"github.com/stackb/centrl/pkg/protoutil"
	sdpb "github.com/stackb/centrl/stardoc_output"
)

type Config struct {
	OutputFile string
	Files      []string
}

func main() {
	log.SetPrefix("documentationcompiler: ")
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return fmt.Errorf("failed to parse args: %w", err)
	}

	var docs bzpb.DocumentationInfo

	for _, file := range cfg.Files {
		var module sdpb.ModuleInfo
		if err := protoutil.ReadFile(file, &module); err != nil {
			return fmt.Errorf("reading %s: %v", file, err)
		}
		fileInfo := parseModuleFile(&module)
		docs.File = append(docs.File, fileInfo)
	}

	if err := protoutil.WriteFile(cfg.OutputFile, &docs); err != nil {
		return fmt.Errorf("failed to write output file: %v", err)
	}

	return nil
}

func parseFlags(args []string) (cfg Config, err error) {
	fs := flag.NewFlagSet("documentationcompiler", flag.ExitOnError)
	fs.StringVar(&cfg.OutputFile, "output_file", "", "the output file to write")

	if err = fs.Parse(args); err != nil {
		return
	}

	cfg.Files = fs.Args()

	if cfg.OutputFile == "" {
		return cfg, fmt.Errorf("output_file is required")
	}

	if len(cfg.Files) == 0 {
		return cfg, fmt.Errorf("at least one asset is required")
	}

	return
}

func parseModuleFile(module *sdpb.ModuleInfo) *bzpb.FileInfo {
	label := parseLabel(module.File)

	fileInfo := &bzpb.FileInfo{
		Label:       label,
		Symbol:      parseModuleSymbols(module),
		Description: truncate(module.ModuleDocstring),
	}

	return fileInfo
}

func parseModuleSymbols(module *sdpb.ModuleInfo) []*bzpb.SymbolInfo {
	var symbols []*bzpb.SymbolInfo

	// Process rules
	for _, rule := range module.RuleInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_RULE,
			Name:        rule.RuleName,
			Description: truncate(rule.DocString),
			Info:        &bzpb.SymbolInfo_Rule{Rule: rule},
		})
	}

	// Process functions
	for _, fn := range module.FuncInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_FUNCTION,
			Name:        fn.FunctionName,
			Description: truncate(fn.DocString),
			Info:        &bzpb.SymbolInfo_Func{Func: fn},
		})
	}

	// Process providers
	for _, provider := range module.ProviderInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_PROVIDER,
			Name:        provider.ProviderName,
			Description: truncate(provider.DocString),
			Info:        &bzpb.SymbolInfo_Provider{Provider: provider},
		})
	}

	// Process aspects
	for _, aspect := range module.AspectInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_ASPECT,
			Name:        aspect.AspectName,
			Description: truncate(aspect.DocString),
			Info:        &bzpb.SymbolInfo_Aspect{Aspect: aspect},
		})
	}

	// Process module extensions
	for _, ext := range module.ModuleExtensionInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_MODULE_EXTENSION,
			Name:        ext.ExtensionName,
			Description: truncate(ext.DocString),
			Info:        &bzpb.SymbolInfo_ModuleExtension{ModuleExtension: ext},
		})
	}

	// Process repository rules
	for _, repoRule := range module.RepositoryRuleInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_REPOSITORY_RULE,
			Name:        repoRule.RuleName,
			Description: truncate(repoRule.DocString),
			Info:        &bzpb.SymbolInfo_RepositoryRule{RepositoryRule: repoRule},
		})
	}

	// Process macros
	for _, macro := range module.MacroInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_MACRO,
			Name:        macro.MacroName,
			Description: truncate(macro.DocString),
			Info:        &bzpb.SymbolInfo_Macro{Macro: macro},
		})
	}

	return symbols
}

// parseLabel parses a Bazel label string into its components
func parseLabel(labelStr string) *bzpb.Label {
	l, err := label.Parse(labelStr)
	if err != nil {
		// If parsing fails, return empty label
		return &bzpb.Label{}
	}

	return &bzpb.Label{
		Repo: l.Repo,
		Pkg:  l.Pkg,
		Name: l.Name,
	}
}

// truncate truncates a string to 64 characters
func truncate(s string) string {
	return s
	// const maxLen = 72
	// if len(s) <= maxLen {
	// 	return s
	// }
	// return s[:maxLen]
}
