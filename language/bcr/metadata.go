package bcr

import (
	"fmt"
	"log"

	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	"github.com/stackb/centrl/pkg/protoutil"
)

func readMetadataJson(filename string) (*bzpb.Metadata, error) {
	var md bzpb.Metadata
	if err := protoutil.ReadFile(filename, &md); err != nil {
		return nil, fmt.Errorf("reading metadata json: %v", err)
	}
	return &md, nil
}

func makeModuleMaintainerRules(maintainers []*bzpb.Metadata_Maintainer) []*rule.Rule {
	var rules []*rule.Rule
	for i, m := range maintainers {
		name := fmt.Sprintf("maintainer_%d", i)
		if m.Github != "" {
			name = m.Github
		} else if m.Email != "" {
			// Use email prefix as name if no github username
			for j, c := range m.Email {
				if c == '@' {
					name = m.Email[:j]
					break
				}
			}
		}

		r := rule.NewRule("module_maintainer", name)
		if m.Email != "" {
			r.SetAttr("email", m.Email)
		}
		if m.Name != "" {
			r.SetAttr("username", m.Name)
		}
		if m.Github != "" {
			r.SetAttr("github", m.Github)
		}
		if m.DoNotNotify {
			r.SetAttr("do_not_notify", m.DoNotNotify)
		}
		if m.GithubUserId != 0 {
			r.SetAttr("github_user_id", int(m.GithubUserId))
		}
		rules = append(rules, r)
	}
	return rules
}

func makeModuleMetadataRule(name string, md *bzpb.Metadata, maintainerRules []*rule.Rule) *rule.Rule {
	r := rule.NewRule("module_metadata", name)
	if md.Homepage != "" {
		r.SetAttr("homepage", md.Homepage)
	}
	if len(maintainerRules) > 0 {
		maintainers := make([]string, len(maintainerRules))
		for i, mr := range maintainerRules {
			maintainers[i] = fmt.Sprintf(":%s", mr.Name())
		}
		r.SetAttr("maintainers", maintainers)
	}
	if len(md.Repository) > 0 {
		r.SetAttr("repository", md.Repository)
	}
	if len(md.Versions) > 0 {
		r.SetAttr("versions", md.Versions)
	}
	if len(md.YankedVersions) > 0 {
		r.SetAttr("yanked_versions", md.YankedVersions)
	}
	if md.Deprecated != "" {
		r.SetAttr("deprecated", md.Deprecated)
	}
	r.SetAttr("visibility", []string{"//visibility:public"})
	return r
}

func moduleMetadataLoadInfo() rule.LoadInfo {
	return rule.LoadInfo{
		Name:    "@centrl//rules:module_metadata.bzl",
		Symbols: []string{"module_metadata"},
	}
}

func moduleMaintainerLoadInfo() rule.LoadInfo {
	return rule.LoadInfo{
		Name:    "@centrl//rules:module_maintainer.bzl",
		Symbols: []string{"module_maintainer"},
	}
}

func moduleMetadataKinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		"module_metadata": {
			MatchAny:     true,
			ResolveAttrs: map[string]bool{"maintainers": true, "deps": true},
		},
		"module_maintainer": {
			MatchAttrs: []string{"name", "email"},
		},
	}
}

// resolveModuleMetadataRule resolves the deps attribute for a module_metadata rule
// by looking up module_version rules for each version in the versions list
func resolveModuleMetadataRule(r *rule.Rule, ix *resolve.RuleIndex, from label.Label) {
	// Get the versions attribute
	versions := r.AttrStrings("versions")
	if len(versions) == 0 {
		return
	}

	moduleName := r.Name()

	// Resolve each version to its module_version rule
	deps := make([]string, 0, len(versions))
	for _, version := range versions {
		// Construct the import spec: "module_name@version"
		importSpec := resolve.ImportSpec{
			Lang: "bcr",
			Imp:  fmt.Sprintf("%s@%s", moduleName, version),
		}

		// Find the module_version rule that provides this import
		results := ix.FindRulesByImport(importSpec, "bcr")

		if len(results) == 0 {
			log.Printf("No module_version found for %s@%s in module_metadata", moduleName, version)
			continue
		}

		// Use the first result (should only be one)
		result := results[0]
		deps = append(deps, result.Label.String())
	}

	// Set the deps attr
	if len(deps) > 0 {
		r.SetAttr("deps", deps)
	}
}
