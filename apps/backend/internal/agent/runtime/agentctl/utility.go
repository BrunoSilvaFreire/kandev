package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

// doLongRunningJSON posts a JSON body to the given path on the long-running
// HTTP client and decodes a JSON response body into out. It is used by the
// utility endpoints (inference prompt, probe) where the underlying LLM call
// or agent cold-start can take minutes.
func (c *Client) doLongRunningJSON(ctx context.Context, path, label string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.longRunningHTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s failed with status %d: %s", label, resp.StatusCode, truncateBody(respBody))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to parse %s response (status %d, body: %s): %w", label, resp.StatusCode, truncateBody(respBody), err)
	}
	return nil
}

// InferencePrompt executes a one-shot inference prompt via agentctl.
// Uses the long-running HTTP client since LLM inference can take several minutes.
// When a progress reporter is attached to ctx it streams NDJSON progress frames;
// otherwise the response is one JSON body, exactly as before.
func (c *Client) InferencePrompt(ctx context.Context, req *utility.PromptRequest) (*utility.PromptResponse, error) {
	reporter := utility.ProgressReporterFrom(ctx)
	if reporter == nil {
		var result utility.PromptResponse
		if err := c.doLongRunningJSON(ctx, "/api/v1/inference/prompt", "utility prompt", req, &result); err != nil {
			return nil, err
		}
		return &result, nil
	}
	return c.inferencePromptStream(ctx, req, reporter)
}

// inferencePromptStream requests the NDJSON form and parses progress frames
// plus the final result. An agentctl that predates streaming ignores the flag
// and returns one JSON body, which parseInferenceStream still accepts.
func (c *Client) inferencePromptStream(
	ctx context.Context,
	req *utility.PromptRequest,
	reporter utility.ProgressReporter,
) (*utility.PromptResponse, error) {
	streamReq := *req
	streamReq.StreamProgress = true
	body, err := json.Marshal(&streamReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/inference/prompt", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/x-ndjson")

	resp, err := c.longRunningHTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := readResponseBody(resp)
		return nil, fmt.Errorf("utility prompt failed with status %d: %s", resp.StatusCode, truncateBody(respBody))
	}
	return parseInferenceStream(resp.Body, reporter)
}

// parseInferenceStream reads NDJSON progress frames and the final result frame.
// The first non-empty line decides the format: an envelope carrying `progress`
// or `result` means NDJSON; anything else is treated as a legacy single JSON
// body and parsed whole.
func parseInferenceStream(r io.Reader, reporter utility.ProgressReporter) (*utility.PromptResponse, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	envelope := false
	var legacy []byte
	for scanner.Scan() {
		line := append([]byte(nil), bytes.TrimSpace(scanner.Bytes())...)
		if len(line) == 0 {
			continue
		}
		if !envelope {
			if legacy != nil {
				legacy = append(legacy, line...)
				continue
			}
			var probe map[string]json.RawMessage
			if err := json.Unmarshal(line, &probe); err != nil {
				// Not JSON on its own: a legacy pretty-printed body. Join the
				// remaining lines and parse the whole document at the end.
				legacy = append(legacy, line...)
				continue
			}
			_, hasProgress := probe["progress"]
			_, hasResult := probe["result"]
			if !hasProgress && !hasResult {
				return decodeLegacyPromptResponse(line)
			}
			envelope = true
		}
		var frame struct {
			Progress *utility.PromptProgress `json:"progress"`
			Result   *utility.PromptResponse `json:"result"`
		}
		if err := json.Unmarshal(line, &frame); err != nil {
			return nil, fmt.Errorf("failed to parse progress frame: %w", err)
		}
		if frame.Progress != nil {
			reporter(*frame.Progress)
		}
		if frame.Result != nil {
			return frame.Result, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read progress stream: %w", err)
	}
	if legacy != nil {
		return decodeLegacyPromptResponse(legacy)
	}
	return nil, fmt.Errorf("inference stream ended without a result")
}

func decodeLegacyPromptResponse(data []byte) (*utility.PromptResponse, error) {
	var result utility.PromptResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse utility prompt response: %w", err)
	}
	return &result, nil
}

// Probe runs an ACP handshake (initialize + session/new) against the agent
// to discover its capabilities, auth methods, models, and modes.
func (c *Client) Probe(ctx context.Context, req *utility.ProbeRequest) (*utility.ProbeResponse, error) {
	var result utility.ProbeResponse
	if err := c.doLongRunningJSON(ctx, "/api/v1/inference/probe", "probe", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
