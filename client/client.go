package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type noteRequest struct {
	Path        string                 `json:"path"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body,omitempty"`
}

type noteResponse struct {
	Path        string                 `json:"path"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Body        string                 `json:"body,omitempty"`
	Created     bool                   `json:"created,omitempty"`
	Updated     bool                   `json:"updated,omitempty"`
	Deleted     bool                   `json:"deleted,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{},
	}
}

func (c *Client) doJSON(method, path string, reqBody interface{}) (*noteResponse, error) {
	var bodyBuf bytes.Buffer
	if reqBody != nil {
		if err := json.NewEncoder(&bodyBuf).Encode(reqBody); err != nil {
			return nil, fmt.Errorf("failed to encode request: %w", err)
		}
	}

	req, err := http.NewRequest(method, c.baseURL+path, &bodyBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var noteResp noteResponse
	if err := json.NewDecoder(resp.Body).Decode(&noteResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return &noteResp, fmt.Errorf("API error (%d): %s", resp.StatusCode, noteResp.Error)
	}

	return &noteResp, nil
}

func (c *Client) Create(notePath string, frontmatter map[string]interface{}, body string) error {
	_, err := c.doJSON("POST", "/api/v1/notes", noteRequest{
		Path:        notePath,
		Frontmatter: frontmatter,
		Body:        body,
	})
	return err
}

func (c *Client) Read(notePath string) (map[string]interface{}, string, error) {
	resp, err := c.doJSON("GET", "/api/v1/notes?path="+url.QueryEscape(notePath), nil)
	if err != nil {
		return nil, "", err
	}
	return resp.Frontmatter, resp.Body, nil
}

func (c *Client) Update(notePath string, frontmatter map[string]interface{}, body string) error {
	_, err := c.doJSON("PUT", "/api/v1/notes", noteRequest{
		Path:        notePath,
		Frontmatter: frontmatter,
		Body:        body,
	})
	return err
}

func (c *Client) Delete(notePath string) error {
	_, err := c.doJSON("DELETE", "/api/v1/notes", noteRequest{
		Path: notePath,
	})
	return err
}
