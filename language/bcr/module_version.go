package bcr

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
)

const (
	isLatestVersionPrivateAttr = "_is_latest_version"
	moduleVersionPrivateAttr   = "_module_version"
)

func makeModuleVersionKey(name, version string) string {
	return fmt.Sprintf("%s@%s", name, version)
}

func parseModuleVersionKey(key string) (string, string) {
	parts := strings.SplitN(key, "@", 2)
	return parts[0], parts[1]
}

// moduleVersionLoadInfo returns load info for the module_version rule
func moduleVersionLoadInfo() rule.LoadInfo {
	return rule.LoadInfo{
		Name:    "//rules:module_version.bzl",
		Symbols: []string{"module_version"},
	}
}

// moduleVersionKinds returns kind info for the module_version rule
func moduleVersionKinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		"module_version": {
			MatchAny: true,
			ResolveAttrs: map[string]bool{
				"deps":         true,
				"source":       true,
				"attestations": true,
				"presubmit":    true,
				"commit":       true,
			},
		},
	}
}

// makeModuleVersionRule creates a module_version rule from parsed MODULE.bazel data
func makeModuleVersionRule(module *bzpb.ModuleVersion, version string, depRules []*rule.Rule, sourceRule *rule.Rule, attestationsRule *rule.Rule, presubmitRule *rule.Rule, commitRule *rule.Rule, moduleBazelFile string) *rule.Rule {
	r := rule.NewRule("module_version", version)
	if module.Name != "" {
		r.SetAttr("module_name", module.Name)
	}
	if version != "" {
		r.SetAttr("version", version)
	}
	if module.CompatibilityLevel != 0 {
		r.SetAttr("compatibility_level", int(module.CompatibilityLevel))
	}
	if len(module.BazelCompatibility) > 0 {
		r.SetAttr("bazel_compatibility", module.BazelCompatibility)
	}
	if module.RepoName != "" {
		r.SetAttr("repo_name", module.RepoName)
	}
	if len(depRules) > 0 {
		deps := make([]string, len(depRules))
		for i, dr := range depRules {
			deps[i] = fmt.Sprintf(":%s", dr.Name())
		}
		r.SetAttr("deps", deps)
	}
	if sourceRule != nil {
		r.SetAttr("source", fmt.Sprintf(":%s", sourceRule.Name()))
	}
	if attestationsRule != nil {
		r.SetAttr("attestations", fmt.Sprintf(":%s", attestationsRule.Name()))
	}
	if presubmitRule != nil {
		r.SetAttr("presubmit", fmt.Sprintf(":%s", presubmitRule.Name()))
	}
	if commitRule != nil {
		r.SetAttr("commit", fmt.Sprintf(":%s", commitRule.Name()))
	}
	if moduleBazelFile != "" {
		r.SetAttr("module_bazel", moduleBazelFile)
	}
	r.SetAttr("build_bazel", ":BUILD.bazel")
	r.SetAttr("visibility", []string{"//visibility:public"})

	// Set GazelleImports private attr with the import spec
	// The import spec for a module version is "module_name@version"
	if module.Name != "" && version != "" {
		importSpec := makeModuleVersionKey(module.Name, version)
		r.SetPrivateAttr(config.GazelleImportsKey, []string{importSpec})
	}

	r.SetPrivateAttr(moduleVersionPrivateAttr, module)

	return r
}

// moduleVersionImports returns import specs for indexing module_version rules
func moduleVersionImports(r *rule.Rule) []resolve.ImportSpec {
	// Get the module name and version to construct the import spec
	moduleName := r.AttrString("module_name")
	version := r.AttrString("version")

	if moduleName == "" || version == "" {
		return nil
	}

	// Construct and return the import spec: "module_name@version"
	importSpec := resolve.ImportSpec{
		Lang: bcrLangName,
		Imp:  makeModuleVersionKey(moduleName, version),
	}

	return []resolve.ImportSpec{importSpec}
}

func resolveModuleVersionRule(r *rule.Rule, moduleRules map[string]*rule.Rule) {
	moduleName := r.AttrString("module_name")
	moduleVersion := r.AttrString("version")

	if moduleRule, ok := moduleRules[moduleName]; !ok {
		// https://github.com/bazelbuild/bazel-central-registry/tree/8c5761038905a45f1cf2d1098ba9917a456d20bb/modules/postgres/14.18
		log.Printf("WARN: while resolving latest versions, discovered unknown module: %v", moduleName)
	} else {
		versions := moduleRule.AttrStrings("versions")

		// latest version is expected to be the last element in the list
		if len(versions) > 0 && versions[len(versions)-1] == moduleVersion {
			r.SetAttr("is_latest_version", true)
			r.SetPrivateAttr(isLatestVersionPrivateAttr, true)
		}
	}
}

// updateModuleVersionRulePublishedDocs sets the published_docs attribute on the
// module_version rule corresponding to the given module_source rule
func updateModuleVersionRulePublishedDocs(moduleSourceRule *rule.Rule, httpArchiveLabel label.Label, moduleVersions map[string]*rule.Rule) {
	// Get the module@version from the module_source rule's private attr
	module := moduleSourceRule.PrivateAttr(moduleVersionPrivateAttr).(*bzpb.ModuleVersion)

	// Look up the corresponding module_version rule using ext.moduleVersions map
	moduleKey := makeModuleVersionKey(module.Name, module.Version)
	if moduleVersionRule, exists := moduleVersions[moduleKey]; exists {
		// Set the published_docs attribute as a label_list
		if httpArchiveLabel != label.NoLabel {
			moduleVersionRule.SetAttr("published_docs", []string{httpArchiveLabel.String()})
		}
	} else {
		log.Panicf("BUG: not module version found for %s", moduleKey)
	}
}

func updateModuleVersionRulesMvs(moduleVersions map[string]*rule.Rule, attrName string, perModuleVersionMvs mvs) (annotatedCount int) {
	for moduleKey, mvs := range perModuleVersionMvs {
		// Find the corresponding module_version rule
		rule, exists := moduleVersions[moduleKey]
		if !exists {
			continue
		}

		// Extract root module name and version to exclude from mvs attribute
		parts := strings.Split(moduleKey, "@")
		if len(parts) != 2 {
			continue
		}
		rootModuleName := parts[0]

		// Remove root module from the mvs dict (we only want dependencies)
		mvsWithoutRoot := make(map[string]string)
		for moduleName, version := range mvs {
			if moduleName != rootModuleName {
				mvsWithoutRoot[moduleName] = version
			}
		}

		// Set the "mvs" attribute as a dict (without root module)
		if len(mvsWithoutRoot) > 0 {
			rule.SetAttr(attrName, mvsWithoutRoot)
			annotatedCount++
		}
	}

	return
}

func hasStarlarkLanguage(moduleMetadataRule *rule.Rule, repositoryMetadataByCanonicalName map[string]*bzpb.RepositoryMetadata) bool {
	// Get the repository field
	repositories := moduleMetadataRule.AttrStrings("repository")
	if len(repositories) == 0 {
		return false
	}

	// Check if the repositoriy has Starlark in its languages
	for _, repo := range repositories {
		canonicalName := repositoryMetadataCanonicalName(repo)
		repoMetadata, exists := repositoryMetadataByCanonicalName[canonicalName]
		if !exists {
			continue
		}
		if repoMetadata.Languages == nil {
			continue
		}
		if _, hasLang := repoMetadata.Languages["Starlark"]; hasLang {
			return true
		}
	}

	return false
}

func isLatestVersion(moduleVersionRule *rule.Rule) bool {
	isLatest, ok := moduleVersionRule.PrivateAttr(isLatestVersionPrivateAttr).(bool)
	return ok && isLatest
}

func selectVersion(version string, available []*versionedRule, metadata *bzpb.ModuleMetadata) label.Label {
	if len(available) == 0 {
		return label.NoLabel
	}
	upvote := func(v *versionedRule) label.Label {
		v.rank++
		return v.label
	}
	for _, v := range available {
		if v.version == version {
			return upvote(v)
		}
	}

	return upvote(available[len(available)-1])
}

func (ext *bcrExtension) updateModuleVersionRuleBzlSrcsAndDeps(moduleKey string, mvs map[string]string, starlarkRepositories moduleVersionRuleMap) bool {
	// skip setting bzl_srcs and deps on non-latest versions
	mvRule, exists := ext.moduleVersionRulesByModuleKey[moduleKey]
	if !exists {
		return false
	}
	if !isLatestVersion(mvRule) {
		return false
	}

	rootModuleName, rootModuleVersion := parseModuleVersionKey(moduleKey)

	// Separate root from dependencies
	var bzlSrcLabel label.Label
	var bzlDepLabels []label.Label

	for moduleName, version := range mvs {
		moduleMetadataRule, exists := ext.moduleMetadataRulesByModuleName[rootModuleName]
		if !exists {
			return false
		}
		if !hasStarlarkLanguage(moduleMetadataRule, ext.repositoriesMetadataByCanonicalName) {
			continue
		}

		metadata := moduleMetadataRule.PrivateAttr(moduleMetadataPrivateAttr).(*bzpb.ModuleMetadata)
		selectedVersion := selectVersion(version, starlarkRepositories[moduleName], metadata)

		if moduleName == rootModuleName && version == rootModuleVersion {
			// This is the root module → bzl_srcs (single label)
			bzlSrcLabel = selectedVersion
		} else {
			// This is a dependency → bzl_deps (list)
			bzlDepLabels = append(bzlDepLabels, selectedVersion)
		}
	}

	// Only set attributes if we have a root bzl_srcs
	if bzlSrcLabel == label.NoLabel {
		return false
	}

	// Set bzl_srcs attribute using select expression
	mvRule.SetAttr("bzl_srcs", makeBzlSrcSelectExpr(bzlSrcLabel.String()))

	// Set bzl_deps attribute if there are any dependencies
	if len(bzlDepLabels) > 0 {
		bzlDeps := make([]string, 0, len(bzlDepLabels))
		for _, bzlDepLabel := range bzlDepLabels {
			if bzlDepLabel != label.NoLabel {
				bzlDeps = append(bzlDeps, bzlDepLabel.String())
			}
		}
		sort.Strings(bzlDeps)
		mvRule.SetAttr("bzl_deps", makeBzlDepsSelectExpr(bzlDeps))
	}

	return true
}

func (ext *bcrExtension) updateModuleVersionRulesBzlSrcsAndDeps(perModuleVersionMvs mvs, starlarkRepositories moduleVersionRuleMap) (annotatedCount int) {
	for moduleKey, mvs := range perModuleVersionMvs {
		if ext.updateModuleVersionRuleBzlSrcsAndDeps(moduleKey, mvs, starlarkRepositories) {
			annotatedCount++
		}
	}

	log.Printf("Annotated %d module_version rules with bzl_srcs/bzl_deps", annotatedCount)
	return
}
