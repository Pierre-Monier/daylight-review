// ABOUTME: GitLab REST API client for the five operations needed by the assignment pipeline
// ABOUTME: GitLabClient interface allows the pipeline to be tested with a mock
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Note struct {
	Body string `json:"body"`
}

type GitLabClient interface {
	MRNotes(ctx context.Context, projectID, mrIID string) ([]Note, error)
	// MRChanges returns the list of changed file paths and the MR author's username.
	MRChanges(ctx context.Context, projectID, mrIID string) (files []string, authorUsername string, err error)
	CODEOWNERSContent(ctx context.Context, projectID, ref string) (string, error)
	SetReviewers(ctx context.Context, projectID, mrIID string, usernames []string) error
	PostNote(ctx context.Context, projectID, mrIID, body string, confidential bool) error
}

var _ GitLabClient = (*Client)(nil)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) MRNotes(ctx context.Context, projectID, mrIID string) ([]Note, error) {
	// GitLab returns notes newest-first; 100 per page is the max. Pagination is not
	// implemented — on MRs with >100 notes the idempotency note could fall off page 1.
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%s/notes?per_page=100", c.baseURL, projectID, mrIID)
	var notes []Note
	if err := c.get(ctx, url, &notes); err != nil {
		return nil, err
	}
	return notes, nil
}

type mrChangesResponse struct {
	Author struct {
		Username string `json:"username"`
	} `json:"author"`
	Changes []struct {
		NewPath string `json:"new_path"`
	} `json:"changes"`
}

func (c *Client) MRChanges(ctx context.Context, projectID, mrIID string) ([]string, string, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%s/changes", c.baseURL, projectID, mrIID)
	var resp mrChangesResponse
	if err := c.get(ctx, url, &resp); err != nil {
		return nil, "", err
	}
	files := make([]string, 0, len(resp.Changes))
	for _, ch := range resp.Changes {
		files = append(files, ch.NewPath)
	}
	return files, resp.Author.Username, nil
}

func (c *Client) CODEOWNERSContent(ctx context.Context, projectID, ref string) (string, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/CODEOWNERS/raw?ref=%s", c.baseURL, projectID, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitLab API status %d fetching CODEOWNERS", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

type userResponse struct {
	ID int `json:"id"`
}

func (c *Client) SetReviewers(ctx context.Context, projectID, mrIID string, usernames []string) error {
	ids := make([]int, 0, len(usernames))
	for _, username := range usernames {
		id, err := c.userID(ctx, username)
		if err != nil {
			return fmt.Errorf("lookup user %s: %w", username, err)
		}
		ids = append(ids, id)
	}
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%s", c.baseURL, projectID, mrIID)
	return c.doJSON(ctx, http.MethodPut, url, map[string]any{"reviewer_ids": ids}, http.StatusOK)
}

func (c *Client) userID(ctx context.Context, username string) (int, error) {
	url := fmt.Sprintf("%s/api/v4/users?username=%s", c.baseURL, username)
	var users []userResponse
	if err := c.get(ctx, url, &users); err != nil {
		return 0, err
	}
	if len(users) == 0 {
		return 0, fmt.Errorf("user not found: %s", username)
	}
	return users[0].ID, nil
}

func (c *Client) PostNote(ctx context.Context, projectID, mrIID, body string, confidential bool) error {
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%s/notes", c.baseURL, projectID, mrIID)
	return c.doJSON(ctx, http.MethodPost, url,
		map[string]any{"body": body, "confidential": confidential},
		http.StatusCreated)
}

func (c *Client) get(ctx context.Context, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitLab API status %d for GET %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

func (c *Client) doJSON(ctx context.Context, method, url string, payload any, expectedStatus int) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("GitLab API status %d for %s %s", resp.StatusCode, method, url)
	}
	return nil
}
