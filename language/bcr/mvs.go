package bcr

import (
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/bazelbuild/buildtools/build"
	"github.com/dominikbraun/graph"
)

type mvs map[string]map[string]string

// calculateMvs implements Minimum Version Selection algorithm
// This calculates MVS for each individual module@version in the registry
func (ext *bcrExtension) calculateMvs(starlarkRepositories moduleVersionRuleMap) {
	log.Println("Running Minimum Version Selection (MVS) algorithm...")

	// allVersions maps module name -> all available versions (sorted)
	allVersions := ext.extractAllVersions()
	// perModuleVersionMvs maps "module@version" -> (module name -> selected
	// version) This shows what MVS would select for regular deps if that
	// specific module@version were the root
	perModuleVersionMvs := ext.calculatePerModuleVersionMvs(allVersions, ext.regularDepGraph, "regular")
	// perModuleVersionMvsDev maps "module@version" -> (module name -> selected
	// version) This shows what MVS would select for dev deps if that specific
	// module@version were the root
	perModuleVersionMvsDev := ext.calculatePerModuleVersionMvs(allVersions, ext.devDepGraph, "dev")
	// perModuleVersionMvsMerged records selected versions in the merged set of
	// regular + dev
	// perModuleVersionMvsMerged := ext.calculatePerModuleVersionMvs(allVersions, ext.depGraph, "merged")

	// Annotate module_version rules with their MVS results
	updateModuleVersionRulesMvs(ext.moduleVersionRulesByModuleKey, "mvs", perModuleVersionMvs)
	updateModuleVersionRulesMvs(ext.moduleVersionRulesByModuleKey, "mvs_dev", perModuleVersionMvsDev)

	ext.updateModuleVersionRulesBzlSrcsAndDeps(perModuleVersionMvs, starlarkRepositories)
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
		if ext.unresolvedModulesByModuleName[moduleKey] {
			skippedUnresolved++
			continue
		}

		moduleName, _ := parseModuleVersionKey(moduleKey)
		moduleNames[moduleName] = true
	}

	if skippedUnresolved > 0 {
		log.Printf("MVS: Skipped %d unresolved module versions from graph", skippedUnresolved)
	}

	// Get sorted versions from module_metadata for each module
	skippedNoMetadata := 0
	for moduleName := range moduleNames {
		metadataRule, exists := ext.moduleMetadataRulesByModuleName[moduleName]
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
func (ext *bcrExtension) calculatePerModuleVersionMvs(allVersions map[string][]string, depGraph graph.Graph[string, string], depType string) mvs {
	perModuleVersionMvs := make(mvs)

	// Get all module@version nodes from the graph
	adjacencyMap, err := depGraph.AdjacencyMap()
	if err != nil {
		log.Printf("Error getting adjacency map for per-version MVS (%s): %v", depType, err)
		return perModuleVersionMvs
	}

	// Collect module keys to process (excluding unresolved)
	var moduleKeys []string
	for moduleKey := range adjacencyMap {
		if !ext.unresolvedModulesByModuleName[moduleKey] {
			moduleKeys = append(moduleKeys, moduleKey)
		}
	}

	if len(moduleKeys) == 0 {
		log.Println("No module versions to calculate MVS for")
		return perModuleVersionMvs
	}

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
				selected := runMvs([]string{moduleKey}, allVersions, adjacencyMap)
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
	}

	log.Printf("\nCalculated MVS for %d module versions", len(moduleKeys))
	return perModuleVersionMvs
}

// runMvs runs the MVS algorithm starting from root modules or module@version keys
// roots can be either:
//   - module names (e.g., "bazel_skylib") - selects highest version
//   - module@version keys (e.g., "bazel_skylib@1.8.2") - uses that specific version
//
// adjacencyMap is passed in to avoid repeated fetches
// Returns the selected version for each module (excluding the roots themselves)
func runMvs(roots []string, allVersions map[string][]string, adjacencyMap map[string]map[string]graph.Edge[string]) map[string]string {
	selected := make(map[string]string)
	var moduleKeys []string

	// Process roots and determine starting module@version keys
	for _, root := range roots {
		// Check if root is already a module@version key (contains '@')
		if strings.Contains(root, "@") {
			// It's a module@version key, use it directly
			moduleName, version := parseModuleVersionKey(root)
			selected[moduleName] = version
			moduleKeys = append(moduleKeys, root)
		} else {
			// It's a module name, select the highest version
			versions := allVersions[root]
			if len(versions) == 0 {
				log.Panicf("BUG: root module %q has no versions in allVersions", root)
			}
			highestVersion := versions[len(versions)-1]
			selected[root] = highestVersion
			moduleKeys = append(moduleKeys, makeModuleVersionKey(root, highestVersion))
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

		moduleName, version := parseModuleVersionKey(moduleKey)

		// Update selected version if this is higher
		if currentVersion, exists := selected[moduleName]; !exists || compareVersions(version, currentVersion) > 0 {
			selected[moduleName] = version
		}

		// Visit dependencies using adjacency map
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

	return selected
}

// calculatePerModuleVersionMergedMvs computes the union of regular and dev MVS
// results for a set of module@version(s) Returns a map of module name ->
// selected version foreach one.
func (ext *bcrExtension) calculatePerModuleVersionMergedMvs(allVersions map[string][]string, deps, devDeps mvs) mvs {
	merged := make(mvs)

	// TODO: implement this

	return merged
}

// narrowMvsVersions reduces multiple versions of the same module to keep only
// the highest patch version (comparing the last dotted element if present)
// This narrows within the same major.minor version family
// Input map can have keys in two formats:
// 1. "moduleName" -> "version" (original format)
// 2. "moduleName@version" -> "version" (format with embedded module name)
func narrowMvsVersions(mvs map[string]string) map[string]string {
	if len(mvs) == 0 {
		return mvs
	}

	// First, normalize the input to extract module names and versions
	type moduleVersion struct {
		module  string
		version string
	}
	var entries []moduleVersion

	for key, version := range mvs {
		// Check if key contains @ (format: module@version -> version)
		if strings.Contains(key, "@") {
			// Extract module name from key
			module := key[:strings.Index(key, "@")]
			entries = append(entries, moduleVersion{module: module, version: version})
		} else {
			// Key is module name, value is version
			entries = append(entries, moduleVersion{module: key, version: version})
		}
	}

	// Track the best version for each module@baseVersion
	type versionInfo struct {
		module   string
		patchNum int
		version  string
	}
	bestVersions := make(map[string]*versionInfo) // key: module@major.minor

	for _, entry := range entries {
		moduleName := entry.module
		version := entry.version

		// Find the last dot in the version string
		lastDotIdx := strings.LastIndex(version, ".")
		if lastDotIdx == -1 {
			// No dot, keep as-is (no narrowing possible)
			key := moduleName + "@nodot@" + version
			bestVersions[key] = &versionInfo{
				module:   moduleName,
				patchNum: -1,
				version:  version,
			}
			continue
		}

		// Split into base and patch
		baseVersion := version[:lastDotIdx]
		patchStr := version[lastDotIdx+1:]

		// Try to parse patch as int
		patchNum, err := strconv.Atoi(patchStr)
		if err != nil {
			// Not a numeric patch, keep as-is (no narrowing possible)
			key := moduleName + "@nonnumeric@" + version
			bestVersions[key] = &versionInfo{
				module:   moduleName,
				patchNum: -1,
				version:  version,
			}
			continue
		}

		// Group by module@baseVersion (e.g., "aspect_bazel_lib@1.8")
		key := moduleName + "@" + baseVersion
		existing, exists := bestVersions[key]
		if !exists || patchNum > existing.patchNum {
			bestVersions[key] = &versionInfo{
				module:   moduleName,
				patchNum: patchNum,
				version:  version,
			}
		}
	}

	// Build result: module name -> best version
	result := make(map[string]string)

	for _, info := range bestVersions {
		// Keep the highest version for each module
		if existingVersion, exists := result[info.module]; exists {
			if compareVersions(info.version, existingVersion) > 0 {
				result[info.module] = info.version
			}
		} else {
			result[info.module] = info.version
		}
	}

	return result
}

// makeBzlSrcSelectExpr creates a select expression for the bzl_srcs attribute (single label)
//
//	Returns: select({
//	    "//app/bcr:is_docs_release": "label",
//	    "//conditions:default": None,
//	})
func makeBzlSrcSelectExpr(label string) *build.CallExpr {
	return &build.CallExpr{
		X: &build.Ident{Name: "select"},
		List: []build.Expr{
			&build.DictExpr{
				List: []*build.KeyValueExpr{
					{
						Key:   &build.StringExpr{Value: "//app/bcr:is_docs_release"},
						Value: &build.StringExpr{Value: label},
					},
					{
						Key:   &build.StringExpr{Value: "//conditions:default"},
						Value: &build.Ident{Name: "None"},
					},
				},
			},
		},
	}
}

// makeBzlDepsSelectExpr creates a select expression for the bzl_deps attribute (list of labels)
//
//	Returns: select({
//	    "//app/bcr:is_docs_release": [labels...],
//	    "//conditions:default": [],
//	})
func makeBzlDepsSelectExpr(labels []string) *build.CallExpr {
	// Sort labels for consistent output
	sortedLabels := make([]string, len(labels))
	copy(sortedLabels, labels)
	slices.Sort(sortedLabels)

	// Create list of label string expressions
	labelExprs := make([]build.Expr, 0, len(sortedLabels))
	for _, lbl := range sortedLabels {
		labelExprs = append(labelExprs, &build.StringExpr{Value: lbl})
	}

	return &build.CallExpr{
		X: &build.Ident{Name: "select"},
		List: []build.Expr{
			&build.DictExpr{
				List: []*build.KeyValueExpr{
					{
						Key: &build.StringExpr{Value: "//app/bcr:is_docs_release"},
						Value: &build.ListExpr{
							List: labelExprs,
						},
					},
					{
						Key:   &build.StringExpr{Value: "//conditions:default"},
						Value: &build.ListExpr{List: []build.Expr{}},
					},
				},
			},
		},
	}
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
