package bcr

import (
	"context"
	"log"
)

// Before is called before any other lifecycle methods.
// This can be used to initialize resources needed during the build.
func (ext *bcrExtension) Before(ctx context.Context) {
	// Nothing to initialize before processing
	log.Println("===[Before]======================================")
}

// DoneGeneratingRules is called after all rules have been generated.
// This is the ideal place to detect circular dependencies since the
// complete dependency graph has been built.
func (ext *bcrExtension) DoneGeneratingRules() {
	// Get all detected cycles
	cycles := ext.getCycles()

	// Build the module-to-cycle mapping for use during resolution
	ext.moduleToCycle = buildModuleToCycleMap(cycles)

	// Log any circular dependencies
	ext.logCycles()

	log.Println("===[DoneGeneratingRules]======================================")
}

// AfterResolvingDeps is called after all dependencies have been resolved.
// This can be used to clean up resources or perform final validation.
func (ext *bcrExtension) AfterResolvingDeps(ctx context.Context) {
	// Nothing to clean up after resolution
	log.Println("===[AfterResolvingDeps]======================================")
}
