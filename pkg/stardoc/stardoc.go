package stardoc

import (
	sympb "github.com/stackb/centrl/build/stack/bazel/symbol/v1"
	slpb "github.com/stackb/centrl/build/stack/starlark/v1beta1"
	sdpb "github.com/stackb/centrl/stardoc_output"
)

// ModuleInfoToFile converts a stardoc ModuleInfo into a File message
func ModuleInfoToFile(module *sdpb.ModuleInfo) *sympb.File {
	return &sympb.File{
		Label:       ParseLabel(module.File),
		Symbol:      makeSymbolsFromModuleInfo(module),
		Description: processDocString(module.ModuleDocstring),
	}
}

// ModuleToFile converts a slpb.Module to a sympb.File
func ModuleToFile(file *sympb.File, module *slpb.Module) {
	file.Symbol = makeSymbolsFromModule(module)
	file.Description = processDocString(module.ModuleDocstring)
}
