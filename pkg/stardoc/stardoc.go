package stardoc

import (
	docpb "github.com/stackb/centrl/build/stack/bazel/documentation/v1"
	slpb "github.com/stackb/centrl/build/stack/starlark/v1beta1"
	sdpb "github.com/stackb/centrl/stardoc_output"
)

// ModuleInfoToFileInfo converts a stardoc ModuleInfo into a FileInfo message
func ModuleInfoToFileInfo(module *sdpb.ModuleInfo) *docpb.FileInfo {
	return &docpb.FileInfo{
		Label:       ParseLabel(module.File),
		Symbol:      makeSymbolsFromModuleInfo(module),
		Description: processDocString(module.ModuleDocstring),
	}
}

// ModuleToFileInfo converts a slpb.Module to a docpb.FileInfo
func ModuleToFileInfo(file *docpb.FileInfo, module *slpb.Module) {
	file.Symbol = makeSymbolsFromModule(module)
	file.Description = processDocString(module.ModuleDocstring)
}
