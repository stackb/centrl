package stardoc

import (
	"github.com/bazelbuild/bazel-gazelle/label"
	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	slpb "github.com/stackb/centrl/build/stack/starlark/v1beta1"
	sdpb "github.com/stackb/centrl/stardoc_output"
)

// ParseModuleFile converts a stardoc ModuleInfo into a FileInfo message
func ParseModuleFile(module *sdpb.ModuleInfo) *bzpb.FileInfo {
	label := ParseLabel(module.File)

	fileInfo := &bzpb.FileInfo{
		Label:       label,
		Symbol:      ParseModuleSymbols(module),
		Description: Truncate(module.ModuleDocstring),
	}

	return fileInfo
}

// ParseModuleSymbols extracts all symbols from a ModuleInfo
func ParseModuleSymbols(module *sdpb.ModuleInfo) []*bzpb.SymbolInfo {
	var symbols []*bzpb.SymbolInfo

	// Process rules
	for _, rule := range module.RuleInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_RULE,
			Name:        rule.RuleName,
			Description: Truncate(rule.DocString),
			Info:        &bzpb.SymbolInfo_Rule{Rule: rule},
		})
	}

	// Process functions
	for _, fn := range module.FuncInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_FUNCTION,
			Name:        fn.FunctionName,
			Description: Truncate(fn.DocString),
			Info:        &bzpb.SymbolInfo_Func{Func: fn},
		})
	}

	// Process providers
	for _, provider := range module.ProviderInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_PROVIDER,
			Name:        provider.ProviderName,
			Description: Truncate(provider.DocString),
			Info:        &bzpb.SymbolInfo_Provider{Provider: provider},
		})
	}

	// Process aspects
	for _, aspect := range module.AspectInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_ASPECT,
			Name:        aspect.AspectName,
			Description: Truncate(aspect.DocString),
			Info:        &bzpb.SymbolInfo_Aspect{Aspect: aspect},
		})
	}

	// Process module extensions
	for _, ext := range module.ModuleExtensionInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_MODULE_EXTENSION,
			Name:        ext.ExtensionName,
			Description: Truncate(ext.DocString),
			Info:        &bzpb.SymbolInfo_ModuleExtension{ModuleExtension: ext},
		})
	}

	// Process repository rules
	for _, repoRule := range module.RepositoryRuleInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_REPOSITORY_RULE,
			Name:        repoRule.RuleName,
			Description: Truncate(repoRule.DocString),
			Info:        &bzpb.SymbolInfo_RepositoryRule{RepositoryRule: repoRule},
		})
	}

	// Process macros
	for _, macro := range module.MacroInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_MACRO,
			Name:        macro.MacroName,
			Description: Truncate(macro.DocString),
			Info:        &bzpb.SymbolInfo_Macro{Macro: macro},
		})
	}

	return symbols
}

// ParseLabel parses a Bazel label string into its components
func ParseLabel(labelStr string) *bzpb.Label {
	l, err := label.Parse(labelStr)
	if err != nil {
		// If parsing fails, return empty label
		return &bzpb.Label{}
	}
	return ToLabel(l)
}

func ToLabel(l label.Label) *bzpb.Label {
	return &bzpb.Label{
		Repo: l.Repo,
		Pkg:  l.Pkg,
		Name: l.Name,
	}
}

// Truncate truncates a string to a reasonable length
func Truncate(s string) string {
	return s
	// const maxLen = 72
	// if len(s) <= maxLen {
	// 	return s
	// }
	// return s[:maxLen]
}

// ModuleToFileInfo converts a slpb.Module to a bzpb.FileInfo
func ModuleToFileInfo(module *slpb.Module) *bzpb.FileInfo {
	if module == nil {
		return nil
	}

	// If the module has a ModuleInfo, use it to parse symbols
	if module.Info != nil {
		return ParseModuleFile(module.Info)
	}

	// Otherwise create a minimal FileInfo with just the name
	return &bzpb.FileInfo{
		Label: &bzpb.Label{
			Name: module.GetName(),
		},
	}
}
