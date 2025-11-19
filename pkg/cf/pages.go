package cf

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
)

// PagesProject represents a Cloudflare Pages project
type PagesProject struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Subdomain         string `json:"subdomain"`
	ProductionBranch  string `json:"production_branch"`
	CreatedOn         string `json:"created_on"`
}

// Deployment represents a Cloudflare Pages deployment
type Deployment struct {
	ID              string   `json:"id"`
	URL             string   `json:"url"`
	Environment     string   `json:"environment"`
	CreatedOn       string   `json:"created_on"`
	Stage           string   `json:"stage"`
	DeploymentStage string   `json:"deployment_stage"`
	Aliases         []string `json:"aliases"`
}

// CreateDeploymentRequest represents the request to create a deployment
type CreateDeploymentRequest struct {
	Branch string `json:"branch,omitempty"`
}

// GetProject retrieves a Cloudflare Pages project
func (c *Client) GetProject(projectName string) (*PagesProject, error) {
	path := fmt.Sprintf("/accounts/%s/pages/projects/%s", c.accountID, projectName)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var project PagesProject
	if err := json.Unmarshal(resp.Result, &project); err != nil {
		return nil, fmt.Errorf("failed to unmarshal project: %w", err)
	}

	return &project, nil
}

// CreateProject creates a new Cloudflare Pages project
func (c *Client) CreateProject(name, productionBranch string) (*PagesProject, error) {
	path := fmt.Sprintf("/accounts/%s/pages/projects", c.accountID)

	req := map[string]interface{}{
		"name":              name,
		"production_branch": productionBranch,
	}

	resp, err := c.doRequest("POST", path, req)
	if err != nil {
		return nil, err
	}

	var project PagesProject
	if err := json.Unmarshal(resp.Result, &project); err != nil {
		return nil, fmt.Errorf("failed to unmarshal project: %w", err)
	}

	return &project, nil
}

// UploadDeployment uploads files to Cloudflare Pages using Direct Upload
func (c *Client) UploadDeployment(projectName string, tarballPath string) (*Deployment, error) {
	// Read tarball
	tarData, err := readTarball(tarballPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read tarball: %w", err)
	}

	// Create manifest - map of file paths to their SHA-256 hashes
	manifest := make(map[string]string)
	for filename, content := range tarData {
		hash := sha256.Sum256(content)
		manifest[filename] = hex.EncodeToString(hash[:])
	}

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add required branch field
	if err := writer.WriteField("branch", "main"); err != nil {
		return nil, fmt.Errorf("failed to write branch field: %w", err)
	}

	// Add manifest field (required by Cloudflare Pages API)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}
	if err := writer.WriteField("manifest", string(manifestJSON)); err != nil {
		return nil, fmt.Errorf("failed to write manifest field: %w", err)
	}

	// Add files from tarball - each file as a separate form field with the hash as the field name
	for filename, content := range tarData {
		hash := sha256.Sum256(content)
		hashStr := hex.EncodeToString(hash[:])

		part, err := writer.CreateFormFile(hashStr, filename)
		if err != nil {
			return nil, fmt.Errorf("failed to create form file for %s: %w", filename, err)
		}
		if _, err := part.Write(content); err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", filename, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Upload to Cloudflare Pages
	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments",
		apiBaseURL, c.accountID, projectName)

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to upload: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal upload response: %w", err)
	}

	if !apiResp.Success {
		if len(apiResp.Errors) > 0 {
			return nil, apiResp.Errors[0]
		}
		return nil, fmt.Errorf("upload failed without error details")
	}

	var deployment Deployment
	if err := json.Unmarshal(apiResp.Result, &deployment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deployment: %w", err)
	}

	return &deployment, nil
}

// readTarball reads a tarball and returns a map of filename to content
func readTarball(path string) (map[string][]byte, error) {
	file, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	files := make(map[string][]byte)
	tr := tar.NewReader(file)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}

		// Read file content
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s from tar: %w", header.Name, err)
		}

		// Clean up filename (remove leading ./)
		filename := strings.TrimPrefix(header.Name, "./")
		files[filename] = content
	}

	return files, nil
}

// openFile opens a file for reading (helper for testing)
var openFile = func(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

// GetDeployment retrieves information about a specific deployment
func (c *Client) GetDeployment(projectName, deploymentID string) (*Deployment, error) {
	path := fmt.Sprintf("/accounts/%s/pages/projects/%s/deployments/%s",
		c.accountID, projectName, deploymentID)

	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var deployment Deployment
	if err := json.Unmarshal(resp.Result, &deployment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deployment: %w", err)
	}

	return &deployment, nil
}

// ListDeployments lists all deployments for a project
func (c *Client) ListDeployments(projectName string) ([]Deployment, error) {
	path := fmt.Sprintf("/accounts/%s/pages/projects/%s/deployments",
		c.accountID, projectName)

	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var deployments []Deployment
	if err := json.Unmarshal(resp.Result, &deployments); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deployments: %w", err)
	}

	return deployments, nil
}
