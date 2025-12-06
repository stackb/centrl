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
// Deprecated: Use ParseModuleSymbolsWithLocation for symbols with location information
func ParseModuleSymbols(module *sdpb.ModuleInfo) []*bzpb.SymbolInfo {
	var symbols []*bzpb.SymbolInfo

	// Process rules
	for _, rule := range module.RuleInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_RULE,
			Name:        rule.RuleName,
			Description: Truncate(rule.DocString),
			Info:        &bzpb.SymbolInfo_Rule{Rule: &slpb.Rule{Info: rule}},
		})
	}

	// Process functions
	for _, fn := range module.FuncInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_FUNCTION,
			Name:        fn.FunctionName,
			Description: Truncate(fn.DocString),
			Info:        &bzpb.SymbolInfo_Func{Func: &slpb.Function{Info: fn}},
		})
	}

	// Process providers
	for _, provider := range module.ProviderInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_PROVIDER,
			Name:        provider.ProviderName,
			Description: Truncate(provider.DocString),
			Info:        &bzpb.SymbolInfo_Provider{Provider: &slpb.Provider{Info: provider}},
		})
	}

	// Process aspects
	for _, aspect := range module.AspectInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_ASPECT,
			Name:        aspect.AspectName,
			Description: Truncate(aspect.DocString),
			Info:        &bzpb.SymbolInfo_Aspect{Aspect: &slpb.Aspect{Info: aspect}},
		})
	}

	// Process module extensions
	for _, ext := range module.ModuleExtensionInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_MODULE_EXTENSION,
			Name:        ext.ExtensionName,
			Description: Truncate(ext.DocString),
			Info:        &bzpb.SymbolInfo_ModuleExtension{ModuleExtension: &slpb.ModuleExtension{Info: ext}},
		})
	}

	// Process repository rules
	for _, repoRule := range module.RepositoryRuleInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_REPOSITORY_RULE,
			Name:        repoRule.RuleName,
			Description: Truncate(repoRule.DocString),
			Info:        &bzpb.SymbolInfo_RepositoryRule{RepositoryRule: &slpb.RepositoryRule{Info: repoRule}},
		})
	}

	// Process macros
	for _, macro := range module.MacroInfo {
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_MACRO,
			Name:        macro.MacroName,
			Description: Truncate(macro.DocString),
			Info:        &bzpb.SymbolInfo_Macro{Macro: &slpb.Macro{Info: macro}},
		})
	}

	return symbols
}

// ParseModuleSymbolsWithLocation extracts all symbols from a slpb.Module with location information
func ParseModuleSymbolsWithLocation(module *slpb.Module) []*bzpb.SymbolInfo {
	if module == nil {
		return nil
	}

	var symbols []*bzpb.SymbolInfo

	// Build a map of symbol names to locations for quick lookup
	locationMap := make(map[string]*slpb.SymbolLocation)
	for _, loc := range module.SymbolLocation {
		locationMap[loc.Name] = loc
	}

	// If module has Info, process legacy types (rules, functions, providers, aspects)
	if module.Info != nil {
		// Process rules
		for _, rule := range module.Info.RuleInfo {
			symbols = append(symbols, &bzpb.SymbolInfo{
				Type:        bzpb.SymbolType_SYMBOL_TYPE_RULE,
				Name:        rule.RuleName,
				Description: Truncate(rule.DocString),
				Info: &bzpb.SymbolInfo_Rule{Rule: &slpb.Rule{
					Info:     rule,
					Location: locationMap[rule.RuleName],
				}},
			})
		}

		// Process functions
		for _, fn := range module.Info.FuncInfo {
			symbols = append(symbols, &bzpb.SymbolInfo{
				Type:        bzpb.SymbolType_SYMBOL_TYPE_FUNCTION,
				Name:        fn.FunctionName,
				Description: Truncate(fn.DocString),
				Info: &bzpb.SymbolInfo_Func{Func: &slpb.Function{
					Info:     fn,
					Location: locationMap[fn.FunctionName],
				}},
			})
		}

		// Process providers
		for _, provider := range module.Info.ProviderInfo {
			symbols = append(symbols, &bzpb.SymbolInfo{
				Type:        bzpb.SymbolType_SYMBOL_TYPE_PROVIDER,
				Name:        provider.ProviderName,
				Description: Truncate(provider.DocString),
				Info: &bzpb.SymbolInfo_Provider{Provider: &slpb.Provider{
					Info:     provider,
					Location: locationMap[provider.ProviderName],
				}},
			})
		}

		// Process aspects
		for _, aspect := range module.Info.AspectInfo {
			symbols = append(symbols, &bzpb.SymbolInfo{
				Type:        bzpb.SymbolType_SYMBOL_TYPE_ASPECT,
				Name:        aspect.AspectName,
				Description: Truncate(aspect.DocString),
				Info: &bzpb.SymbolInfo_Aspect{Aspect: &slpb.Aspect{
					Info:     aspect,
					Location: locationMap[aspect.AspectName],
				}},
			})
		}
	}

	// Process repository rules (these already have locations in the Module)
	for _, repoRule := range module.RepositoryRule {
		name := ""
		if repoRule.Info != nil {
			name = repoRule.Info.RuleName
		}
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_REPOSITORY_RULE,
			Name:        name,
			Description: Truncate(repoRule.Info.GetDocString()),
			Info:        &bzpb.SymbolInfo_RepositoryRule{RepositoryRule: repoRule},
		})
	}

	// Process module extensions (these already have locations in the Module)
	for _, ext := range module.ModuleExtension {
		name := ""
		if ext.Info != nil {
			name = ext.Info.ExtensionName
		}
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_MODULE_EXTENSION,
			Name:        name,
			Description: Truncate(ext.Info.GetDocString()),
			Info:        &bzpb.SymbolInfo_ModuleExtension{ModuleExtension: ext},
		})
	}

	// Process macros (these already have locations in the Module)
	for _, macro := range module.Macro {
		name := ""
		if macro.Info != nil {
			name = macro.Info.MacroName
		}
		symbols = append(symbols, &bzpb.SymbolInfo{
			Type:        bzpb.SymbolType_SYMBOL_TYPE_MACRO,
			Name:        name,
			Description: Truncate(macro.Info.GetDocString()),
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

	var label *bzpb.Label
	var description string

	// Extract label from module.Info if available
	if module.Info != nil {
		label = ParseLabel(module.Info.File)
		description = Truncate(module.Info.ModuleDocstring)
	} else {
		// Otherwise create a minimal label with just the name
		label = &bzpb.Label{
			Name: module.GetName(),
		}
	}

	fileInfo := &bzpb.FileInfo{
		Label:       label,
		Symbol:      ParseModuleSymbolsWithLocation(module),
		Description: description,
	}

	return fileInfo
}
