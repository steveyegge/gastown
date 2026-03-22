// Package ollama provides a client for the Ollama local LLM API.
package ollama

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const DefaultBaseURL = "http://localhost:11434"

// Client communicates with a local Ollama instance.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient returns a Client pointed at the default Ollama endpoint.
func NewClient() *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Ping checks that the Ollama server is reachable.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL, nil)
	if err != nil {
		return fmt.Errorf("creating ping request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("pinging ollama: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status from ollama: %d", resp.StatusCode)
	}
	return nil
}

// Model is a single entry returned by /api/tags.
type Model struct {
	Name string `json:"name"`
}

// ListModelsResponse is the response from /api/tags.
type ListModelsResponse struct {
	Models []Model `json:"models"`
}

// ListModels returns the models available on the Ollama server.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("creating list request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}
	defer resp.Body.Close()
	var result ListModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding models: %w", err)
	}
	return result.Models, nil
}

// GenerateRequest is the request body for /api/generate.
type GenerateRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Stream bool     `json:"stream"`
	Images []string `json:"images,omitempty"` // base64-encoded images
}

// GenerateResponse is the (non-streaming) response from /api/generate.
type GenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Generate sends a text prompt (and optional images) to a model and returns
// the complete response. Streaming is disabled.
func (c *Client) Generate(ctx context.Context, model, prompt string, images ...[]byte) (*GenerateResponse, error) {
	gr := GenerateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}
	for _, img := range images {
		gr.Images = append(gr.Images, base64.StdEncoding.EncodeToString(img))
	}
	body, err := json.Marshal(gr)
	if err != nil {
		return nil, fmt.Errorf("marshaling generate request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating generate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("generate returned %d: %s", resp.StatusCode, string(b))
	}

	var result GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding generate response: %w", err)
	}
	return &result, nil
}
