package bcr

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/bazelbuild/buildtools/build"
	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	"github.com/stackb/centrl/pkg/modulebazel"
	"github.com/stackb/centrl/pkg/netutil"
	"github.com/stackb/centrl/pkg/protoutil"
)

const (
	docsRepoSuffix                   = "_docs"
	starlarkRepositoryRootTargetName = "bzl_srcs"
)

type checkItem struct {
	url   string
	rules []*rule.Rule
}

// trackDocsUrl keeps a list of rules that reference this doc URL.
func (ext *bcrExtension) trackDocsUrl(url string, moduleSource *rule.Rule) {
	if url == "" || strings.Contains(url, "{OWNER}") || strings.Contains(url, "{REPO}") || strings.Contains(url, "{TAG}") {
		return
	}
	ext.docUrls[url] = append(ext.docUrls[url], moduleSource)
}

func (ext *bcrExtension) trackSourceUrl(url string, moduleSource *rule.Rule) {
	if url == "" {
		return
	}
	ext.sourceUrls[url] = append(ext.sourceUrls[url], moduleSource)
}

// makeDocsRepositories determines what URLs are not broken and then builds
// external repos for them for the sake of creating docs.
func (ext *bcrExtension) makeDocsRepositories(repoRoot string) error {
	docsHttpArchives := ext.makeDocsUrlRepositories()
	docsStarlarkRepositories := ext.makeDocsSourceUrlRepositories()

	if err := mergeModuleBazelFile(repoRoot, docsHttpArchives, docsStarlarkRepositories); err != nil {
		return err
	}

	// Write the updated resource status cache back to file
	return ext.writeResourceStatusFile()
}

// writeResourceStatusFile writes the resource status map back to the file it was loaded from
func (ext *bcrExtension) writeResourceStatusFile() error {
	if ext.resourceStatusSetFile == "" {
		// No file was specified, so nothing to write
		return nil
	}

	// Convert map to ResourceStatusSet
	statusSet := &bzpb.ResourceStatusSet{
		Status: make([]*bzpb.ResourceStatus, 0, len(ext.resourceStatus)),
	}
	urls := slices.Sorted(maps.Keys(ext.resourceStatus))
	for _, url := range urls {
		status := ext.resourceStatus[url]
		statusSet.Status = append(statusSet.Status, status)
	}

	filename := os.ExpandEnv(ext.resourceStatusSetFile)
	if err := protoutil.WriteFile(filename, statusSet); err != nil {
		return fmt.Errorf("failed to write resource status file %s: %w", filename, err)
	}

	log.Printf("Wrote %d resource statuses to %s", len(ext.resourceStatus), filename)
	return nil
}

// handleDocsUrlStatus processes a docs URL status and updates the repos map and rules
func handleDocsUrlStatus(url string, rules []*rule.Rule, status netutil.URLStatus, repos map[label.Label]*rule.Rule, resourceStatus map[string]*bzpb.ResourceStatus, cached bool) {
	// Store status in the map for future caching
	resourceStatus[url] = &bzpb.ResourceStatus{
		Url:     url,
		Code:    int32(status.Code),
		Message: status.Message,
	}

	if status.Exists() {
		httpArchiveLabel := makeDocsHttpArchiveLabel(url)
		docsHttpArchive := makeDocsHttpArchive(httpArchiveLabel, url)
		repos[httpArchiveLabel] = docsHttpArchive
		for _, r := range rules {
			updateModuleSourceRuleDocsUrlStatus(r, httpArchiveLabel, status)
		}
	} else {
		cacheMsg := ""
		if cached {
			cacheMsg = " (cached)"
		}
		log.Printf("warning: docs URL does not exist%s: %s (status: %d %s)", cacheMsg, url, status.Code, status.Message)
		for _, r := range rules {
			updateModuleSourceRuleDocsUrlStatus(r, label.NoLabel, status)
		}
	}
}

func (ext *bcrExtension) makeDocsUrlRepositories() map[label.Label]*rule.Rule {
	if len(ext.docUrls) == 0 {
		return nil
	}

	repos := make(map[label.Label]*rule.Rule)

	// Separate URLs into cached, blacklisted, MVS-filtered, and uncached
	var uncachedItems []checkItem
	var cachedCount int
	var blacklistedCount int
	var mvsFilteredCount int

	for url, rules := range ext.docUrls {
		if ext.blacklistedUrls[url] {
			// Skip blacklisted URLs
			blacklistedCount++
			log.Printf("Skipping blacklisted docs URL: %s", url)
			continue
		}

		// Filter by MVS - only process URLs for selected versions
		if !ext.isUrlForSelectedVersion(rules) {
			mvsFilteredCount++
			continue
		}
		if cachedStatus, found := ext.resourceStatus[url]; found {
			// Use cached status
			cachedCount++
			status := netutil.URLStatus{
				Code:    int(cachedStatus.Code),
				Message: cachedStatus.Message,
			}
			handleDocsUrlStatus(url, rules, status, repos, ext.resourceStatus, true)
		} else {
			// Need to check this URL
			uncachedItems = append(uncachedItems, checkItem{url, rules})
		}
	}

	if cachedCount > 0 {
		log.Printf("Skipped %d cached docs URL checks", cachedCount)
	}
	if blacklistedCount > 0 {
		log.Printf("Skipped %d blacklisted docs URLs", blacklistedCount)
	}
	if mvsFilteredCount > 0 {
		log.Printf("Skipped %d docs URLs (not selected by MVS)", mvsFilteredCount)
	}

	// Check uncached URLs in parallel and update rules with status
	if len(uncachedItems) > 0 {
		netutil.CheckURLsParallel("Checking http_archive URLs", uncachedItems, func(item checkItem) string { return item.url },
			func(item checkItem, status netutil.URLStatus) {
				handleDocsUrlStatus(item.url, item.rules, status, repos, ext.resourceStatus, false)
			})
	}

	return repos
}

// handleSourceUrlStatus processes a source URL status and updates the repos map and rules
func handleSourceUrlStatus(repoRoot, url string, rules []*rule.Rule, status netutil.URLStatus, repos map[label.Label]*rule.Rule, resourceStatus map[string]*bzpb.ResourceStatus, cached bool) {
	// Store status in the map for future caching
	resourceStatus[url] = &bzpb.ResourceStatus{
		Url:     url,
		Code:    int32(status.Code),
		Message: status.Message,
	}

	if status.Exists() {
		indexRule := rules[0]
		module := indexRule.PrivateAttr("_module_version").(*bzpb.ModuleVersion)
		source := indexRule.PrivateAttr("_module_source").(*bzpb.ModuleSource)

		to := makeDocsStarlarkRepositoryBzlSrcsLabel(module.Name, module.Version)
		starlarkRepository := makeDocsStarlarkRepository(repoRoot, module, to, source)
		repos[to] = starlarkRepository

		for _, r := range rules {
			updateModuleSourceRuleUrlStatus(r, to, status)
		}
	} else {
		cacheMsg := ""
		if cached {
			cacheMsg = " (cached)"
		}
		log.Printf("warning: source URL does not exist%s: %s (status: %d %s)", cacheMsg, url, status.Code, status.Message)
		for _, r := range rules {
			updateModuleSourceRuleUrlStatus(r, label.NoLabel, status)
		}
	}
}

func (ext *bcrExtension) makeDocsSourceUrlRepositories() map[label.Label]*rule.Rule {
	if len(ext.sourceUrls) == 0 {
		return nil
	}

	repos := make(map[label.Label]*rule.Rule)

	// Separate URLs into cached, blacklisted, MVS-filtered, and uncached
	var uncachedItems []checkItem
	var cachedCount int
	var blacklistedCount int
	var mvsFilteredCount int

	for url, rules := range ext.sourceUrls {
		if ext.blacklistedUrls[url] {
			// Skip blacklisted URLs
			blacklistedCount++
			log.Printf("Skipping blacklisted source URL: %s", url)
			continue
		}

		// Filter by MVS - only process URLs for selected versions
		if !ext.isUrlForSelectedVersion(rules) {
			mvsFilteredCount++
			continue
		}
		if cachedStatus, found := ext.resourceStatus[url]; found {
			// Use cached status
			cachedCount++
			status := netutil.URLStatus{
				Code:    int(cachedStatus.Code),
				Message: cachedStatus.Message,
			}
			handleSourceUrlStatus(ext.repoRoot, url, rules, status, repos, ext.resourceStatus, true)
		} else {
			// Need to check this URL
			uncachedItems = append(uncachedItems, checkItem{url, rules})
		}
	}

	if cachedCount > 0 {
		log.Printf("Skipped %d cached source URL checks", cachedCount)
	}
	if blacklistedCount > 0 {
		log.Printf("Skipped %d blacklisted source URLs", blacklistedCount)
	}
	if mvsFilteredCount > 0 {
		log.Printf("Skipped %d source URLs (not selected by MVS)", mvsFilteredCount)
	}

	// Check uncached URLs in parallel and update rules with status
	if len(uncachedItems) > 0 {
		netutil.CheckURLsParallel("Checking source URLs", uncachedItems, func(item checkItem) string { return item.url },
			func(item checkItem, status netutil.URLStatus) {
				handleSourceUrlStatus(ext.repoRoot, item.url, item.rules, status, repos, ext.resourceStatus, false)
			})
	}

	return repos
}

// mergeModuleBazelFile updates the MODULE.bazel file with additional rules if
// needed.
func mergeModuleBazelFile(repoRoot string, httpArchives, starlarkRepositories map[label.Label]*rule.Rule) error {
	if len(httpArchives) == 0 && len(starlarkRepositories) == 0 {
		return nil
	}

	filename := filepath.Join(repoRoot, "MODULE.bazel")
	f, err := modulebazel.LoadFile(filename, "")
	if err != nil {
		return fmt.Errorf("parsing: %v", err)
	}

	// clean old rules
	deletedRules := 0
	for _, r := range f.Rules {
		if r.Kind() == "http_archive" || r.Kind() == "starlark_repository.archive" {
			if strings.HasSuffix(r.Name(), docsRepoSuffix) {
				r.Delete()
				deletedRules++
			}
		}
	}
	f.Sync()
	log.Printf("cleaned up %d old rules", deletedRules)

	starlarkRepositoryNames := make([]build.Expr, 0, len(starlarkRepositories))
	for lbl := range starlarkRepositories {
		starlarkRepositoryNames = append(starlarkRepositoryNames, &build.StringExpr{Value: lbl.Repo})
	}

	// update stmts
	for _, stmt := range f.File.Stmt {
		switch call := stmt.(type) {
		case *build.CallExpr:
			useRepo := getUseRepoCall(call, "starlark_repository")
			if useRepo != nil {
				useRepo.List = append([]build.Expr{useRepo.List[0]}, starlarkRepositoryNames...)
				call.ForceMultiLine = true
				log.Printf(`updated use_repo(starlark_repository") with %d names (%d)`, len(starlarkRepositoryNames), len(starlarkRepositories))
				break
			}
		}
	}
	f.Sync()

	for _, r := range httpArchives {
		r.Insert(f)
	}
	for _, r := range starlarkRepositories {
		r.Insert(f)
	}
	f.Sync()

	log.Printf("added %d http_archive{s}", len(httpArchives))
	log.Printf("added %d starlark_repositor{y|ies}", len(starlarkRepositories))

	data := f.Format()
	os.WriteFile(filename, data, 0744)

	log.Println("Updated:", filename)
	return nil
}

func getUseRepoCall(call *build.CallExpr, name string) *build.CallExpr {
	if callName, ok := call.X.(*build.Ident); ok {
		if callName.Name == "use_repo" {
			if len(call.List) > 0 {
				if extName, ok := call.List[0].(*build.Ident); ok {
					if extName.Name == name {
						return call
					}
				}
			}
		}
	}
	return nil
}

// makeDocsHttpArchiveLabel creates a label for external workspace
func makeDocsHttpArchiveLabel(docUrl string) label.Label {
	return label.New(makeDocsHttpArchiveRepoName(docUrl), "", "files")
}

// makeDocsStarlarkRepositoryBzlSrcsLabel creates a label for a
// starlark_repository rule.
func makeDocsStarlarkRepositoryBzlSrcsLabel(moduleName, moduleVersion string) label.Label {
	repo := makeDocsStarlarkRepositoryRepoName(moduleName, moduleVersion)
	return label.New(repo, "", starlarkRepositoryRootTargetName)
}

// makeDocsHttpArchiveRepoName creates a named for the external workspace
func makeDocsStarlarkRepositoryRepoName(moduleName, moduleVersion string) (name string) {
	return fmt.Sprintf("%s_%s%s", moduleName, sanitizeName(moduleVersion), docsRepoSuffix)
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "+", "_")
	return name
}

// makeDocsHttpArchiveRepoName creates a named for the external workspace
func makeDocsHttpArchiveRepoName(docUrl string) (name string) {
	name = strings.TrimPrefix(docUrl, "https://")
	name = sanitizeName(name)
	return name + "_docs"
}

func makeDocsHttpArchive(from label.Label, docUrl string) *rule.Rule {
	r := rule.NewRule("http_archive", from.Repo)
	r.SetAttr("url", docUrl)
	r.SetAttr("build_file_content", fmt.Sprintf(`filegroup(name = "%s",
    srcs = glob(["**/*.binaryproto"]),
    visibility = ["//visibility:public"],
)`, from.Name))
	return r
}

func makeDocsStarlarkRepository(repoRoot string, module *bzpb.ModuleVersion, from label.Label, source *bzpb.ModuleSource) *rule.Rule {
	r := rule.NewRule("starlark_repository.archive", from.Repo)
	r.SetAttr("urls", []string{source.Url})
	r.SetAttr("type", getArchiveTypeOrDefault(source.Url, "tar.gz"))

	if source.StripPrefix != "" {
		r.SetAttr("strip_prefix", source.StripPrefix)
	}
	if source.Integrity != "" && strings.HasPrefix(source.Url, "http:") {
		if sha256, ok := getSha256Integrity(source.Integrity); ok {
			r.SetAttr("sha256", sha256)
		}
	}
	r.SetAttr("build_file_generation", "clean")
	r.SetAttr("languages", []string{"starlarklibrary"})

	directives := []string{
		// fmt.Sprintf("gazelle:starlarklibrary_log_file %s/starlark_repository-gazelle.%s-%s.log", repoRoot, module.Name, module.Version),
		"gazelle:starlarklibrary_root",
	}

	r.SetAttr("build_directives", directives)

	return r
}

// getArchiveTypeOrDefault retuns a default if the url extension is not one of
// the ones recognized by http_archive.
func getArchiveTypeOrDefault(sourceUrl, defaultType string) string {
	// see https://bazel.build/rules/lib/repo/http#http_archive
	allowedTypes := []string{
		".zip", ".jar", ".war", ".aar", ".tar", ".tar.gz", ".tgz",
		".tar.xz", ".txz", ".tar.zst", ".tzst", ".tar.bz2", ".ar", ".deb", ".7z",
	}

	// Try matching from longest to shortest to handle multi-part extensions like .tar.gz
	for _, ext := range allowedTypes {
		if strings.HasSuffix(sourceUrl, ext) {
			// Return without the leading dot
			return strings.TrimPrefix(ext, ".")
		}
	}

	// Default to tar.gz if no recognized extension
	return defaultType
}

func getSha256Integrity(integrity string) (string, bool) {
	// example:
	// integrity = "sha256-ShAT7rtQ9yj8YBvdgzsLKHAzPDs+WoFu66kh2VvsbxU=",

	if !strings.HasPrefix(integrity, "sha256-") {
		return "", false
	}

	// Remove the "sha256-" prefix
	b64Hash := strings.TrimPrefix(integrity, "sha256-")

	// Decode from base64
	hashBytes, err := base64.StdEncoding.DecodeString(b64Hash)
	if err != nil {
		return "", false
	}

	// Convert to hex string
	return hex.EncodeToString(hashBytes), true
}
