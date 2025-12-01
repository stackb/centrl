package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/go-github/v66/github"
	"github.com/shurcooL/githubv4"
	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

// Repo identifies a GitHub repository
type Repo struct {
	Owner string
	Name  string
}

// RepoInfo contains repository metadata
type RepoInfo struct {
	Repo           Repo
	Description    string
	StargazerCount int
	Languages      map[string]int
	Error          error
}

// NewClient creates a new GitHub API client
// If token is empty, creates an unauthenticated client (lower rate limits)
func NewClient(token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	}
	return github.NewClient(nil).WithAuthToken(token)
}

// FetchRepoInfo fetches repository description, stargazer count, and languages using the GitHub API
func FetchRepoInfo(ctx context.Context, client *github.Client, repo Repo) (*RepoInfo, error) {
	// Get repository info (includes description and stargazer count)
	repository, _, err := client.Repositories.Get(ctx, repo.Owner, repo.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repository: %w", err)
	}

	// Get languages
	languages, _, err := client.Repositories.ListLanguages(ctx, repo.Owner, repo.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch languages: %w", err)
	}

	info := &RepoInfo{
		Repo:           repo,
		Description:    repository.GetDescription(),
		StargazerCount: repository.GetStargazersCount(),
		Languages:      languages,
	}

	return info, nil
}

// FetchRepoInfoBatchOptions configures batch fetching behavior
type FetchRepoInfoBatchOptions struct {
	// RequestsPerHour sets the rate limit. Default is 4500 (90% of GitHub's 5000/hour authenticated limit)
	RequestsPerHour float64
	// Burst sets the maximum burst size. Default is 100
	Burst int
}

// DefaultFetchOptions returns default options assuming an authenticated client
func DefaultFetchOptions() *FetchRepoInfoBatchOptions {
	return &FetchRepoInfoBatchOptions{
		RequestsPerHour: 4800, // 96% of GitHub's 5000/hour authenticated limit (留 buffer for safety)
		Burst:           1000, // Allow large initial burst
	}
}

// FetchRepoInfoBatch fetches repository info for multiple repos with rate limiting
// Uses default options assuming authenticated client (4800 requests/hour, burst 1000)
func FetchRepoInfoBatch(ctx context.Context, client *github.Client, repos []Repo) ([]*RepoInfo, error) {
	return FetchRepoInfoBatchWithOptions(ctx, client, repos, DefaultFetchOptions())
}

// FetchRepoInfoBatchWithOptions fetches repository info for multiple repos with custom rate limiting
func FetchRepoInfoBatchWithOptions(ctx context.Context, client *github.Client, repos []Repo, opts *FetchRepoInfoBatchOptions) ([]*RepoInfo, error) {
	if opts == nil {
		opts = DefaultFetchOptions()
	}

	results := make([]*RepoInfo, len(repos))
	var mu sync.Mutex

	limiter := rate.NewLimiter(rate.Limit(opts.RequestsPerHour/3600.0), opts.Burst)

	g, ctx := errgroup.WithContext(ctx)

	for i, repo := range repos {
		i, repo := i, repo // capture loop variables
		g.Go(func() error {
			if err := limiter.Wait(ctx); err != nil {
				return err
			}

			info, err := FetchRepoInfo(ctx, client, repo)
			if err != nil {
				// Store error in result instead of failing entire batch
				info = &RepoInfo{
					Repo:  repo,
					Error: err,
				}
			}

			mu.Lock()
			results[i] = info
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// CommitInfo contains commit metadata
type CommitInfo struct {
	SHA        string
	CommitDate time.Time
	Author     string
	Message    string
}

// FetchCommitInfo fetches commit details from GitHub
func FetchCommitInfo(ctx context.Context, client *github.Client, repo Repo, commitSHA string) (*CommitInfo, error) {
	commit, _, err := client.Repositories.GetCommit(ctx, repo.Owner, repo.Name, commitSHA, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch commit %s: %w", commitSHA, err)
	}

	info := &CommitInfo{
		SHA:        commit.GetSHA(),
		CommitDate: commit.GetCommit().GetCommitter().GetDate().Time,
		Author:     commit.GetCommit().GetAuthor().GetName(),
		Message:    commit.GetCommit().GetMessage(),
	}

	return info, nil
}

// NewGraphQLClient creates a new GitHub GraphQL API client
func NewGraphQLClient(token string) *githubv4.Client {
	if token == "" {
		return githubv4.NewClient(nil)
	}

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	return githubv4.NewClient(httpClient)
}

// FetchRepositoryMetadataBatch fetches repository metadata using GraphQL and populates the proto messages
// Fetches up to 100 repositories in a single GraphQL query
func FetchRepositoryMetadataBatch(ctx context.Context, token string, repos []*bzpb.RepositoryMetadata) error {
	if len(repos) == 0 {
		return nil
	}
	if len(repos) > 100 {
		return fmt.Errorf("maximum 100 repositories per batch, got %d", len(repos))
	}

	// Build the GraphQL query dynamically with aliases for all repos
	var queryBuilder bytes.Buffer
	queryBuilder.WriteString("query {\n")

	for i, repo := range repos {
		if repo.Type != bzpb.RepositoryType_GITHUB {
			continue
		}

		queryBuilder.WriteString(fmt.Sprintf(`  repo%d: repository(owner: "%s", name: "%s") {
    description
    stargazerCount
    languages(first: 10, orderBy: {field: SIZE, direction: DESC}) {
      edges {
        size
        node {
          name
        }
      }
    }
  }
`, i, repo.Organization, repo.Name))
	}

	queryBuilder.WriteString("}\n")

	// Execute raw GraphQL query
	response, err := executeRawGraphQL(ctx, token, queryBuilder.String())
	if err != nil {
		return fmt.Errorf("failed to execute GraphQL query: %w", err)
	}

	// Parse response and populate protos
	return parseRepositoryMetadataResponse(response, repos)
}

// populateRepoMetadata populates a RepositoryMetadata proto from GraphQL response
func populateRepoMetadata(repo *bzpb.RepositoryMetadata, fields interface{}) {
	type LanguageEdge struct {
		Size githubv4.Int
		Node struct {
			Name githubv4.String
		}
	}

	type RepositoryFields struct {
		Description    githubv4.String
		StargazerCount githubv4.Int
		Languages      struct {
			Edges []LanguageEdge
		}
	}

	rf, ok := fields.(RepositoryFields)
	if !ok {
		return
	}

	repo.Description = string(rf.Description)
	repo.Stargazers = int32(rf.StargazerCount)

	// Convert languages
	if len(rf.Languages.Edges) > 0 {
		repo.Languages = make(map[string]int32)
		for _, edge := range rf.Languages.Edges {
			repo.Languages[string(edge.Node.Name)] = int32(edge.Size)
		}
	}
}

// executeRawGraphQL executes a raw GraphQL query against GitHub's API
func executeRawGraphQL(ctx context.Context, token, query string) (map[string]any, error) {
	reqBody := map[string]string{
		"query": query,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/graphql", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GraphQL request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %v", result.Errors)
	}

	return result.Data, nil
}

// parseRepositoryMetadataResponse parses the GraphQL response and populates repo protos
func parseRepositoryMetadataResponse(data map[string]any, repos []*bzpb.RepositoryMetadata) error {
	for i, repo := range repos {
		if repo.Type != bzpb.RepositoryType_GITHUB {
			continue
		}
		canonicalName := formatRepository(repo)
		repoKey := fmt.Sprintf("repo%d", i)
		repoData, ok := data[repoKey].(map[string]any)
		if !ok {
			log.Printf("WARN %s: graphql response repo not found: %s", canonicalName, repoKey)
			continue // Repo not found or error
		}

		// Initialize Languages map to indicate metadata was fetched (even if empty)
		repo.Languages = make(map[string]int32)

		// Parse description (nil is valid for repos without descriptions)
		if desc, ok := repoData["description"].(string); ok {
			repo.Description = desc
		} else if repoData["description"] != nil {
			log.Printf("WARN %s: graphql response description parse issue: %v", canonicalName, repoData["description"])
		}

		// Parse stargazer count
		if stars, ok := repoData["stargazerCount"].(float64); ok {
			repo.Stargazers = int32(stars)
		} else {
			log.Printf("WARN %s: graphql response stargazerCount parse issue: %v", canonicalName, repoData["stargazerCount"])
		}

		// Parse languages
		if languages, ok := repoData["languages"].(map[string]any); ok {
			if edges, ok := languages["edges"].([]any); ok {
				for _, edge := range edges {
					edgeMap, ok := edge.(map[string]any)
					if !ok {
						continue
					}

					size, sizeOk := edgeMap["size"].(float64)
					if !sizeOk {
						log.Printf("WARN %s: graphql response edge size parse issue: %v/%v", canonicalName, edgeMap["size"])
						continue
					}
					node, nodeOk := edgeMap["node"].(map[string]any)
					if !nodeOk {
						log.Printf("WARN %s: graphql response node parse issue: %v/%v", canonicalName, edgeMap["node"])
						continue
					}
					name, nameOk := node["name"].(string)
					if !nameOk {
						log.Printf("WARN %s: graphql response node name parse issue: %v", canonicalName, node["name"])
						continue
					}

					repo.Languages[name] = int32(size)
				}
			} else {
				log.Printf("WARN %s: graphql response edges parse issue: %v", canonicalName, repoData["edges"])
			}
		} else {
			log.Printf("WARN %s: graphql response languages parse issue: %v", canonicalName, repoData["languages"])
		}
	}

	return nil
}

// formatRepository prints a canonical form of a repository string
// e.g., "github:org/repo"
func formatRepository(md *bzpb.RepositoryMetadata) string {
	switch md.Type {
	case bzpb.RepositoryType_GITHUB:
		return fmt.Sprintf("github:%s/%s", md.Organization, md.Name)
	default:
		return fmt.Sprintf("%s/%s", md.Organization, md.Name)
	}
}
