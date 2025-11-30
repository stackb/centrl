package bcr

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/dominikbraun/graph"
	"github.com/schollz/progressbar/v3"
)

// mvsResult holds the result of MVS algorithm - the selected version for each module
type mvsResult struct {
	// selectedVersions maps module name -> selected version (global MVS result)
	selectedVersions map[string]string
	// allVersions maps module name -> all available versions (sorted)
	allVersions map[string][]string
	// perModuleVersionMvs maps "module@version" -> (module name -> selected version)
	// This shows what MVS would select for regular deps if that specific module@version were the root
	perModuleVersionMvs map[string]map[string]string
	// perModuleVersionMvsDev maps "module@version" -> (module name -> selected version)
	// This shows what MVS would select for dev deps if that specific module@version were the root
	perModuleVersionMvsDev map[string]map[string]string
}

// calculateMvs implements Minimum Version Selection algorithm
// This calculates MVS for each individual module@version in the registry
func (ext *bcrExtension) calculateMvs() {
	log.Println("Running Minimum Version Selection (MVS) algorithm...")

	// Extract all module versions from the dependency graph
	allVersions := ext.extractAllVersions()

	// Calculate MVS for each individual module@version (regular deps)
	perModuleVersionMvs := ext.calculatePerModuleVersionMvs(allVersions, ext.regularDepGraph, "regular")

	// Calculate MVS for each individual module@version (dev deps only)
	perModuleVersionMvsDev := ext.calculatePerModuleVersionMvs(allVersions, ext.devDepGraph, "dev")

	// Store the results
	ext.mvsResult = &mvsResult{
		allVersions:            allVersions,
		perModuleVersionMvs:    perModuleVersionMvs,
		perModuleVersionMvsDev: perModuleVersionMvsDev,
		// selectedVersions is calculated on-demand if needed via calculateGlobalMvs()
	}

	// Annotate module_version rules with their MVS results
	ext.annotateModuleVersionsWithMvs()

	// Log the results
	ext.logMvsResults()
}

// extractAllVersions extracts all module versions from the dependency graph
// Returns a map of module name -> sorted list of versions from ModuleMetadata
// Skips modules that are unresolved (no module_version rule in registry)
func (ext *bcrExtension) extractAllVersions() map[string][]string {
	allVersions := make(map[string][]string)

	// Get all modules that appear in the dependency graph
	adjacencyMap, err := ext.depGraph.AdjacencyMap()
	if err != nil {
		log.Panicf("Error getting adjacency map: %v", err)
	}

	// Collect unique module names from the graph, skipping unresolved modules
	moduleNames := make(map[string]bool)
	skippedUnresolved := 0
	for moduleKey := range adjacencyMap {
		// Skip unresolved module@version entries
		if ext.unresolvedModules[moduleKey] {
			skippedUnresolved++
			continue
		}

		moduleName, _ := mustParseModuleKey(moduleKey)
		moduleNames[moduleName] = true
	}

	if skippedUnresolved > 0 {
		log.Printf("MVS: Skipped %d unresolved module versions from graph", skippedUnresolved)
	}

	// Get sorted versions from module_metadata for each module
	skippedNoMetadata := 0
	for moduleName := range moduleNames {
		metadataRule, exists := ext.modules[moduleName]
		if !exists {
			// This can happen if a module has unresolved dependencies but we haven't
			// seen the module itself (only references to it)
			log.Printf("MVS: Skipping module %q (in graph but no module_metadata)", moduleName)
			skippedNoMetadata++
			continue
		}

		versions := metadataRule.AttrStrings("versions")
		if len(versions) == 0 {
			log.Panicf("BUG: module_metadata for %q has no versions", moduleName)
		}

		// metadata.json has versions sorted (oldest to newest)
		allVersions[moduleName] = versions
	}

	if skippedNoMetadata > 0 {
		log.Printf("MVS: Skipped %d modules with no metadata", skippedNoMetadata)
	}

	return allVersions
}

// calculatePerModuleVersionMvs computes MVS for each module@version in the given graph
// Returns map of "module@version" -> (module name -> selected version)
// depGraph is the dependency graph to use (either regular deps or dev deps)
// depType is a description for the progress bar ("regular" or "dev")
func (ext *bcrExtension) calculatePerModuleVersionMvs(allVersions map[string][]string, depGraph graph.Graph[string, string], depType string) map[string]map[string]string {
	perModuleVersionMvs := make(map[string]map[string]string)

	// Get all module@version nodes from the graph
	adjacencyMap, err := depGraph.AdjacencyMap()
	if err != nil {
		log.Printf("Error getting adjacency map for per-version MVS (%s): %v", depType, err)
		return perModuleVersionMvs
	}

	// Collect module keys to process (excluding unresolved)
	var moduleKeys []string
	for moduleKey := range adjacencyMap {
		if !ext.unresolvedModules[moduleKey] {
			moduleKeys = append(moduleKeys, moduleKey)
		}
	}

	if len(moduleKeys) == 0 {
		log.Println("No module versions to calculate MVS for")
		return perModuleVersionMvs
	}

	// Create progress bar
	description := fmt.Sprintf("Calculating MVS (%s deps)", depType)
	bar := progressbar.NewOptions(len(moduleKeys),
		progressbar.OptionSetDescription(description),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	// Parallelize MVS calculations using worker pool
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Create channels for jobs and results
	jobChan := make(chan string, len(moduleKeys))
	resultChan := make(chan struct {
		moduleKey string
		result    map[string]string
	}, len(moduleKeys))

	// Start worker goroutines
	numWorkers := min(10, len(moduleKeys)) // Limit concurrent workers

	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for moduleKey := range jobChan {
				// Run MVS with this single module@version as the root
				selected := ext.runMvs([]string{moduleKey}, allVersions, adjacencyMap)
				resultChan <- struct {
					moduleKey string
					result    map[string]string
				}{moduleKey: moduleKey, result: selected}
			}
		}()
	}

	// Send jobs
	go func() {
		for _, moduleKey := range moduleKeys {
			jobChan <- moduleKey
		}
		close(jobChan)
	}()

	// Close result channel when all workers are done
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results with progress reporting
	for result := range resultChan {
		mu.Lock()
		perModuleVersionMvs[result.moduleKey] = result.result
		mu.Unlock()

		bar.Add(1)
	}

	log.Printf("\nCalculated MVS for %d module versions", len(moduleKeys))
	return perModuleVersionMvs
}

// runMvs runs the MVS algorithm starting from root modules or module@version keys
// roots can be either:
//   - module names (e.g., "bazel_skylib") - selects highest version
//   - module@version keys (e.g., "bazel_skylib@1.8.2") - uses that specific version
// adjacencyMap is passed in to avoid repeated fetches
// Returns the selected version for each module (excluding the roots themselves)
func (ext *bcrExtension) runMvs(roots []string, allVersions map[string][]string, adjacencyMap map[string]map[string]graph.Edge[string]) map[string]string {
	selected := make(map[string]string)
	var moduleKeys []string
	rootModules := make(map[string]bool) // Track root module names to exclude from result

	// Process roots and determine starting module@version keys
	for _, root := range roots {
		// Check if root is already a module@version key (contains '@')
		if strings.Contains(root, "@") {
			// It's a module@version key, use it directly
			moduleName, version := mustParseModuleKey(root)
			selected[moduleName] = version
			rootModules[moduleName] = true
			moduleKeys = append(moduleKeys, root)
		} else {
			// It's a module name, select the highest version
			versions := allVersions[root]
			if len(versions) == 0 {
				log.Panicf("BUG: root module %q has no versions in allVersions", root)
			}
			highestVersion := versions[len(versions)-1]
			selected[root] = highestVersion
			rootModules[root] = true
			moduleKeys = append(moduleKeys, fmt.Sprintf("%s@%s", root, highestVersion))
		}
	}

	// Build the transitive closure of dependencies
	// Start from roots and traverse the graph, selecting maximum versions
	visited := make(map[string]bool)
	var visit func(moduleKey string)

	visit = func(moduleKey string) {
		if visited[moduleKey] {
			return
		}
		visited[moduleKey] = true

		moduleName, version := mustParseModuleKey(moduleKey)

		// Update selected version if this is higher
		if currentVersion, exists := selected[moduleName]; !exists || compareVersions(version, currentVersion) > 0 {
			selected[moduleName] = version
		}

		// Visit dependencies using adjacency map (much faster than Edges())
		if deps, exists := adjacencyMap[moduleKey]; exists {
			for targetKey := range deps {
				visit(targetKey)
			}
		}
	}

	// Visit all root module@version keys (and their transitive dependencies)
	for _, moduleKey := range moduleKeys {
		visit(moduleKey)
	}

	// Remove root modules from the result (we only want dependencies)
	for rootModule := range rootModules {
		delete(selected, rootModule)
	}

	return selected
}

// logMvsResults logs the MVS algorithm results
func (ext *bcrExtension) logMvsResults() {
	if ext.mvsResult == nil {
		return
	}

	// Log per-module-version MVS results
	if ext.mvsResult.perModuleVersionMvs != nil {
		log.Printf("Per-module-version MVS results calculated for %d module versions", len(ext.mvsResult.perModuleVersionMvs))
	}
}

// annotateModuleVersionsWithMvs adds the "mvs" and "mvs_dev" attributes to each module_version rule
// The "mvs" attribute is a dict mapping module names to their selected versions (regular deps)
// The "mvs_dev" attribute is a dict mapping module names to their selected versions (dev deps)
func (ext *bcrExtension) annotateModuleVersionsWithMvs() {
	if ext.mvsResult == nil {
		log.Println("No MVS results to annotate")
		return
	}

	annotatedCount := 0
	annotatedDevCount := 0

	// Annotate with regular deps MVS
	if ext.mvsResult.perModuleVersionMvs != nil {
		for moduleKey, mvs := range ext.mvsResult.perModuleVersionMvs {
			// Find the corresponding module_version rule
			rule, exists := ext.moduleVersions[moduleKey]
			if !exists {
				continue
			}

			// Set the "mvs" attribute as a dict
			if len(mvs) > 0 {
				rule.SetAttr("mvs", mvs)
				annotatedCount++
			}
		}
	}

	// Annotate with dev deps MVS
	if ext.mvsResult.perModuleVersionMvsDev != nil {
		for moduleKey, mvsDev := range ext.mvsResult.perModuleVersionMvsDev {
			// Find the corresponding module_version rule
			rule, exists := ext.moduleVersions[moduleKey]
			if !exists {
				continue
			}

			// Set the "mvs_dev" attribute as a dict
			if len(mvsDev) > 0 {
				rule.SetAttr("mvs_dev", mvsDev)
				annotatedDevCount++
			}
		}
	}

	log.Printf("Annotated %d module_version rules with regular MVS results", annotatedCount)
	log.Printf("Annotated %d module_version rules with dev MVS results", annotatedDevCount)
}

// mustParseModuleKey parses "module@version" string into module name and version
// Panics if the key is not in the correct format
func mustParseModuleKey(key string) (string, string) {
	parts := strings.SplitN(key, "@", 2)
	if len(parts) != 2 {
		log.Panicf("BUG: invalid module key format %q, expected 'module@version'", key)
	}
	return parts[0], parts[1]
}

// compareVersions compares two version strings lexicographically
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
// This is used during MVS graph traversal to select the maximum version
// when multiple versions of the same module are encountered.
func compareVersions(v1, v2 string) int {
	if v1 == v2 {
		return 0
	}
	if v1 < v2 {
		return -1
	}
	return 1
}

// isVersionSelected returns true if the given module@version is selected by MVS
// Since global MVS is no longer computed, this always returns true (no filtering)
func (ext *bcrExtension) isVersionSelected(moduleName, version string) bool {
	// Global MVS filtering disabled - include all versions
	return true
}

// isUrlForSelectedVersion checks if any of the rules associated with a URL
// are for an MVS-selected module version
// Since global MVS is no longer computed, this always returns true (no filtering)
func (ext *bcrExtension) isUrlForSelectedVersion(rules []*rule.Rule) bool {
	// Global MVS filtering disabled - include all URLs
	return true
}

// getMvsResultForModuleVersion returns the MVS result for a specific module@version
// Returns the map of (module name -> selected version) when the given module@version is the root
// Returns nil if MVS hasn't been calculated or the module@version doesn't exist
//
// Example usage:
//   result := ext.getMvsResultForModuleVersion("bazel_skylib@1.8.2")
//   if result != nil {
//       for moduleName, version := range result {
//           log.Printf("  %s -> %s", moduleName, version)
//       }
//   }
func (ext *bcrExtension) getMvsResultForModuleVersion(moduleKey string) map[string]string {
	if ext.mvsResult == nil || ext.mvsResult.perModuleVersionMvs == nil {
		return nil
	}
	return ext.mvsResult.perModuleVersionMvs[moduleKey]
}
