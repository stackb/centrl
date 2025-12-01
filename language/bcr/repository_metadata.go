package bcr

import (
	"context"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	"github.com/stackb/centrl/pkg/gh"
	"github.com/stackb/centrl/pkg/gl"
	"github.com/stackb/centrl/pkg/protoutil"
)

// repositoryMetadataLoadInfo returns load info for the repository_metadata rule
func repositoryMetadataLoadInfo() rule.LoadInfo {
	return rule.LoadInfo{
		Name:    "//rules:repository_metadata.bzl",
		Symbols: []string{"repository_metadata"},
	}
}

// repositoryMetadataKinds returns kind info for the repository_metadata rule
func repositoryMetadataKinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		"repository_metadata": {
			MatchAttrs: []string{"canonical_name"},
			ResolveAttrs: map[string]bool{
				"description":      true,
				"stargazers":       true,
				"primary_language": true,
			},
		},
	}
}

// repositoryMetadataImports returns import specs for indexing module_metadata rules
func repositoryMetadataImports(r *rule.Rule) []resolve.ImportSpec {
	return []resolve.ImportSpec{{
		Lang: bcrLangName,
		Imp:  r.AttrString("canonical_name"),
	}}
}

// makeRepositoryMetadataRule creates a repository_metadata rule from protobuf metadata
func makeRepositoryMetadataRule(md *bzpb.RepositoryMetadata) *rule.Rule {
	ruleName := makeRepositoryMetadataRuleName(md)

	r := rule.NewRule("repository_metadata", ruleName)
	r.SetAttr("canonical_name", formatRepositoryCanonicalName(md))
	r.SetAttr("visibility", []string{"//visibility:public"})

	updateRepositoryMetadataRule(md, r)

	return r
}

// updateRepositoryMetadataRule updates a repository_metadata rule from protobuf metadata
func updateRepositoryMetadataRule(md *bzpb.RepositoryMetadata, r *rule.Rule) {
	if md.Type != bzpb.RepositoryType_REPOSITORY_TYPE_UNKNOWN {
		r.SetAttr("type", strings.ToLower(md.Type.String()))
	}
	if md.Organization != "" {
		r.SetAttr("organization", md.Organization)
	}
	if md.Name != "" {
		r.SetAttr("repo_name", md.Name)
	}
	if md.Description != "" {
		r.SetAttr("description", md.Description)
	}
	if md.Stargazers != 0 {
		r.SetAttr("stargazers", int(md.Stargazers))
	}
	if len(md.Languages) > 0 {
		r.SetAttr("languages", makeStringDict(md.Languages))
		primaryLanguage := computePrimaryLanguage(md.Languages)
		r.SetAttr("primary_language", primaryLanguage)
	}
	r.SetPrivateAttr(repositoryMetadataPrivateAttr, md)
}

// computePrimaryLanguage returns the language with the highest line count
func computePrimaryLanguage(languages map[string]int32) string {
	var maxLang string
	var maxCount int32

	for lang, count := range languages {
		if count > maxCount {
			maxCount = count
			maxLang = lang
		}
	}

	return maxLang
}

// resolveRepositoryMetadataRule updates the rule with metadata attributes after
// data has been fetched for it.
func resolveRepositoryMetadataRule(r *rule.Rule, _ *resolve.RuleIndex, repositories map[string]*bzpb.RepositoryMetadata) {
	if md, ok := repositories[r.AttrString("canonical_name")]; ok {
		updateRepositoryMetadataRule(md, r)
	}
}

func makeStringDict(in map[string]int32) map[string]string {
	if in == nil {
		return nil
	}
	dict := make(map[string]string)
	for k, v := range in {
		dict[k] = strconv.Itoa(int(v))
	}
	return dict
}

func (ext *bcrExtension) reportGithubRateLimits() {
	if ext.githubClient == nil {
		return
	}

	ctx := context.Background()

	// Check API rate limits
	rateLimits, _, err := ext.githubClient.RateLimit.Get(ctx)
	if err != nil {
		log.Printf("warning: failed to get GitHub API rate limits: %v", err)
		return
	}

	core := rateLimits.GetCore()
	log.Printf("GitHub REST API rate limit: %d remaining of %d (resets at %v)",
		core.Remaining,
		core.Limit,
		core.Reset.Time)

	graphql := rateLimits.GetGraphQL()
	log.Printf("GitHub GraphQL API rate limit: %d remaining of %d (resets at %v)",
		graphql.Remaining,
		graphql.Limit,
		graphql.Reset.Time)
}

func filterGitlabRepositories(repositories map[string]*bzpb.RepositoryMetadata) []*bzpb.RepositoryMetadata {
	names := slices.Sorted(maps.Keys(repositories))

	todo := make([]*bzpb.RepositoryMetadata, 0)
	for _, name := range names {
		md := repositories[name]
		if md == nil || md.Type != bzpb.RepositoryType_GITLAB {
			continue
		}

		// Skip repositories that already have metadata (from cache)
		// Check if Languages map is initialized, which indicates metadata was fetched
		if md.Languages != nil {
			continue
		}

		todo = append(todo, md)
	}
	return todo
}

func filterGithubRepositories(repositories map[string]*bzpb.RepositoryMetadata) []*bzpb.RepositoryMetadata {
	names := slices.Sorted(maps.Keys(repositories))

	todo := make([]*bzpb.RepositoryMetadata, 0)
	for _, name := range names {
		md := repositories[name]
		if md == nil || md.Type != bzpb.RepositoryType_GITHUB {
			continue
		}

		// Skip known bad repos that don't exist
		if md.Organization == "bazel-contrib" && md.Name == "rules_pex" {
			continue
		}

		// Skip repositories that already have metadata (from cache)
		// Check if Languages map is initialized, which indicates metadata was fetched
		if md.Languages != nil {
			continue
		}

		todo = append(todo, md)
	}
	return todo
}

func (ext *bcrExtension) fetchGithubRepositoryMetadata(todo []*bzpb.RepositoryMetadata) {
	if len(todo) == 0 {
		log.Printf("No repositories need metadata fetching")
		return
	}

	if ext.githubClient == nil {
		log.Printf("No github client available, skipping retrieval of github metadata...")
		return
	}

	ext.reportGithubRateLimits()

	log.Printf("Need to fetch metadata for %d repositories", len(todo))

	// Process in batches of 100 (GitHub GraphQL max)
	batchSize := 100
	totalFetched := 0

	ctx := context.Background()

	for i := 0; i < len(todo); i += batchSize {
		end := min(i+batchSize, len(todo))

		batch := todo[i:end]
		log.Printf("Fetching metadata for batch %d-%d of %d repositories using GraphQL...", i+1, end, len(todo))

		// Retry with exponential backoff
		maxRetries := 3
		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				backoff := time.Duration(attempt) * time.Second
				log.Printf("Retrying batch %d-%d after %v (attempt %d/%d)...", i+1, end, backoff, attempt+1, maxRetries)
				time.Sleep(backoff)
			}

			err = gh.FetchRepositoryMetadataBatch(ctx, ext.githubToken, batch)
			if err == nil {
				break
			}

			log.Printf("warning: failed to fetch repository metadata batch (attempt %d/%d): %v", attempt+1, maxRetries, err)
		}

		if err != nil {
			log.Printf("error: failed to fetch repository metadata batch after %d attempts, skipping batch %d-%d", maxRetries, i+1, end)
			continue
		}

		// Log what we fetched
		batchFetched := 0
		for _, md := range batch {
			if md.Description != "" {
				batchFetched++
			}
		}
		totalFetched += batchFetched

		log.Printf("Successfully fetched metadata for %d repositories in this batch", batchFetched)
	}

	log.Printf("Successfully fetched metadata for %d of %d repositories total", totalFetched, len(todo))

	if totalFetched > 0 {
		ext.fetchedRepositoryMetadata = true
	}
}

func (ext *bcrExtension) fetchGitlabRepositoryMetadata(todo []*bzpb.RepositoryMetadata) {
	if len(todo) == 0 {
		log.Printf("No GitLab repositories need metadata fetching")
		return
	}

	if ext.gitlabToken == "" {
		log.Printf("No GitLab token provided, rate limits may apply")
	}

	log.Printf("Need to fetch metadata for %d GitLab repositories", len(todo))

	// Process in batches of 100 (GitLab GraphQL max)
	batchSize := 100
	totalFetched := 0

	ctx := context.Background()

	for i := 0; i < len(todo); i += batchSize {
		end := min(i+batchSize, len(todo))

		batch := todo[i:end]
		log.Printf("Fetching metadata for batch %d-%d of %d GitLab repositories using GraphQL...", i+1, end, len(todo))
		for _, repo := range batch {
			log.Printf(" > %s/%s", repo.Organization, repo.Name)
		}

		// Retry with exponential backoff
		maxRetries := 3
		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				backoff := time.Duration(attempt) * time.Second
				log.Printf("Retrying GitLab batch %d-%d after %v (attempt %d/%d)...", i+1, end, backoff, attempt+1, maxRetries)
				time.Sleep(backoff)
			}

			err = gl.FetchRepositoryMetadataBatch(ctx, ext.gitlabToken, batch)
			if err == nil {
				break
			}

			log.Printf("warning: failed to fetch GitLab repository metadata batch (attempt %d/%d): %v", attempt+1, maxRetries, err)
		}

		if err != nil {
			log.Printf("error: failed to fetch GitLab repository metadata batch after %d attempts, skipping batch %d-%d", maxRetries, i+1, end)
			continue
		}

		// Log what we fetched
		batchFetched := 0
		for _, md := range batch {
			if md.Description != "" {
				batchFetched++
			}
		}
		totalFetched += batchFetched

		log.Printf("Successfully fetched metadata for %d GitLab repositories in this batch", batchFetched)
	}

	log.Printf("Successfully fetched metadata for %d of %d GitLab repositories total", totalFetched, len(todo))

	if totalFetched > 0 {
		ext.fetchedRepositoryMetadata = true
	}
}

// writeResourceStatusCacheFile writes the resource status map back to the file it was loaded from
func (ext *bcrExtension) writeResourceStatusCacheFile() error {
	if ext.resourceStatusSetFile == "" {
		// No file was specified, so nothing to write
		return nil
	}

	// Convert map to ResourceStatusSet
	statusSet := &bzpb.ResourceStatusSet{
		Status: make([]*bzpb.ResourceStatus, 0, len(ext.resourceStatusByUrl)),
	}
	urls := slices.Sorted(maps.Keys(ext.resourceStatusByUrl))
	for _, url := range urls {
		status := ext.resourceStatusByUrl[url]
		statusSet.Status = append(statusSet.Status, status)
	}

	filename := os.ExpandEnv(ext.resourceStatusSetFile)
	if err := protoutil.WriteFile(filename, statusSet); err != nil {
		return fmt.Errorf("failed to write resource status file %s: %w", filename, err)
	}

	log.Printf("Wrote %d resource statuses to %s", len(ext.resourceStatusByUrl), filename)
	return nil
}

// writeRepositoryMetadataCacheFile writes the repository metadata map back to the file it was loaded from
// Only writes if we actually fetched new metadata during this run
func (ext *bcrExtension) writeRepositoryMetadataCacheFile() error {
	if ext.repositoryMetadataSetFile == "" {
		// No file was specified, so nothing to write
		return nil
	}

	if !ext.fetchedRepositoryMetadata {
		// No new metadata was fetched, don't overwrite the cache
		log.Printf("No new repository metadata fetched, skipping write to %s", ext.repositoryMetadataSetFile)
		return nil
	}

	// Convert map to RepositoryMetadataSet
	metadataSet := &bzpb.RepositoryMetadataSet{
		RepositoryMetadata: make([]*bzpb.RepositoryMetadata, 0, len(ext.repositoriesMetadataByCanonicalName)),
	}
	keys := slices.Sorted(maps.Keys(ext.repositoriesMetadataByCanonicalName))
	for _, key := range keys {
		md := ext.repositoriesMetadataByCanonicalName[key]
		metadataSet.RepositoryMetadata = append(metadataSet.RepositoryMetadata, md)
	}

	filename := os.ExpandEnv(ext.repositoryMetadataSetFile)
	if err := protoutil.WriteFile(filename, metadataSet); err != nil {
		return fmt.Errorf("failed to write repository metadata file %s: %w", filename, err)
	}

	log.Printf("Wrote %d repository metadata entries to %s", len(ext.repositoriesMetadataByCanonicalName), filename)
	return nil
}
