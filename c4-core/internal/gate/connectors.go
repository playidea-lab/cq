package gate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Slack connector
// ---------------------------------------------------------------------------

// SlackConnector sends messages to Slack via an Incoming Webhook URL.
type SlackConnector struct {
	webhookURL string
	httpClient *http.Client
}

// NewSlackConnector creates a SlackConnector that posts to the given webhook URL.
func NewSlackConnector(webhookURL string) *SlackConnector {
	return &SlackConnector{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// SendMessage posts a plain-text message to the specified Slack channel.
// The channel field is advisory when using Incoming Webhooks — Slack routes
// the message based on the webhook configuration.
func (c *SlackConnector) SendMessage(channel, text string) error {
	payload := map[string]string{
		"channel": channel,
		"text":    text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	resp, err := c.httpClient.Post(c.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// GitHub connector
// ---------------------------------------------------------------------------

// GitHubConfig holds GitHub API connection settings.
type GitHubConfig struct {
	// PAT is the Personal Access Token used for authentication.
	PAT string
	// BaseURL allows overriding the GitHub API base (useful for testing with httptest).
	// Defaults to "https://api.github.com".
	BaseURL string
}

// GitHubConnector posts comments to GitHub issues and pull requests.
type GitHubConnector struct {
	cfg        GitHubConfig
	httpClient *http.Client
}

// NewGitHubConnector creates a GitHubConnector with the given configuration.
func NewGitHubConnector(cfg GitHubConfig) *GitHubConnector {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.github.com"
	}
	return &GitHubConnector{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// PostIssueComment posts a comment on the specified issue.
func (c *GitHubConnector) PostIssueComment(owner, repo string, issueNumber int, body string) error {
	return c.postComment(owner, repo, issueNumber, body)
}

// PostPRComment posts a comment on the specified pull request.
// GitHub's REST API uses the same /issues/:number/comments endpoint for both
// issues and pull requests.
func (c *GitHubConnector) PostPRComment(owner, repo string, prNumber int, body string) error {
	return c.postComment(owner, repo, prNumber, body)
}

// postComment is the shared implementation for issue and PR comments.
func (c *GitHubConnector) postComment(owner, repo string, number int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.cfg.BaseURL, owner, repo, number)

	payload := map[string]string{"body": body}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal github payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("create github request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.cfg.PAT != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.PAT)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("github API returned HTTP %d", resp.StatusCode)
	}
	return nil
}
