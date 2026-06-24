package facade

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is the interface the CLI depends on. Keeping it small and injectable
// lets command logic be tested against a fake (see facadetest) with no network.
type Client interface {
	Health(ctx context.Context, base string) (Health, error)
	Status(ctx context.Context, base string) (*Status, error)
	ListModels(ctx context.Context, base string) (*ModelList, error)
	Pull(ctx context.Context, base, model string, onProgress func(PullEvent)) error
	Delete(ctx context.Context, base, model string) error
	SetDefault(ctx context.Context, base, model string) error
	Chat(ctx context.Context, base string, req ChatRequest, onDelta func(string)) error
}

// HTTPClient is the production Client, backed by net/http.
type HTTPClient struct {
	http   *http.Client
	apiKey string
}

// Option configures an HTTPClient.
type Option func(*HTTPClient)

// WithHTTPClient overrides the underlying *http.Client (handy in tests).
func WithHTTPClient(h *http.Client) Option {
	return func(c *HTTPClient) { c.http = h }
}

// WithAPIKey sets a bearer token sent with every request, matching the facade's
// optional API_KEY auth.
func WithAPIKey(key string) Option {
	return func(c *HTTPClient) { c.apiKey = key }
}

// NewClient builds an HTTPClient. The default timeout is generous because chat
// and pull are long-lived streams; per-call contexts provide finer control.
func NewClient(opts ...Option) *HTTPClient {
	c := &HTTPClient{http: &http.Client{Timeout: 0}}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *HTTPClient) newRequest(ctx context.Context, method, base, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return req, nil
}

// Health probes readiness. A short timeout is applied so callers polling for
// boot don't hang on a half-up server.
func (c *HTTPClient) Health(ctx context.Context, base string) (Health, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := c.newRequest(ctx, http.MethodGet, base, "/api/health", nil)
	if err != nil {
		return Health{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Health{}, err
	}
	defer drain(resp)

	var h Health
	_ = json.NewDecoder(resp.Body).Decode(&h)
	if resp.StatusCode != http.StatusOK {
		return h, fmt.Errorf("facade not ready: status %d", resp.StatusCode)
	}
	if h.Status == "" {
		h.Status = "ok"
	}
	h.Ready = true
	return h, nil
}

// Status fetches the aggregate snapshot.
func (c *HTTPClient) Status(ctx context.Context, base string) (*Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := c.newRequest(ctx, http.MethodGet, base, "/api/status", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer drain(resp)
	if err := expectOK(resp); err != nil {
		return nil, err
	}
	var s Status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	return &s, nil
}

// ListModels returns installed models and the default.
func (c *HTTPClient) ListModels(ctx context.Context, base string) (*ModelList, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := c.newRequest(ctx, http.MethodGet, base, "/api/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer drain(resp)
	if err := expectOK(resp); err != nil {
		return nil, err
	}
	var ml ModelList
	if err := json.NewDecoder(resp.Body).Decode(&ml); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}
	return &ml, nil
}

// Pull downloads a model, streaming NDJSON progress to onProgress. It blocks
// until the pull finishes, fails, or ctx is cancelled.
func (c *HTTPClient) Pull(ctx context.Context, base, model string, onProgress func(PullEvent)) error {
	body, _ := json.Marshal(map[string]string{"model": model})
	req, err := c.newRequest(ctx, http.MethodPost, base, "/api/models/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pull failed: %s", readError(resp))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev PullEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // tolerate stray non-JSON lines
		}
		if onProgress != nil {
			onProgress(ev)
		}
		if ev.Failed() {
			if ev.Error != "" {
				return fmt.Errorf("pull failed: %s", ev.Error)
			}
			return fmt.Errorf("pull failed")
		}
		if ev.Done() {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("pull stream: %w", err)
	}
	return nil
}

// Delete removes an installed model.
func (c *HTTPClient) Delete(ctx context.Context, base, model string) error {
	return c.postModelAction(ctx, base, "/api/models/delete", model)
}

// SetDefault marks a model as the instance default.
func (c *HTTPClient) SetDefault(ctx context.Context, base, model string) error {
	return c.postModelAction(ctx, base, "/api/models/default", model)
}

func (c *HTTPClient) postModelAction(ctx context.Context, base, path, model string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	body, _ := json.Marshal(map[string]string{"model": model})
	req, err := c.newRequest(ctx, http.MethodPost, base, path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	return expectOK(resp)
}

// Chat streams an OpenAI-compatible completion, delivering content deltas to
// onDelta as they arrive (SSE). It returns when the stream completes.
func (c *HTTPClient) Chat(ctx context.Context, base string, creq ChatRequest, onDelta func(string)) error {
	creq.Stream = true
	body, _ := json.Marshal(creq)
	req, err := c.newRequest(ctx, http.MethodPost, base, "/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chat failed: %s", readError(resp))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return fmt.Errorf("chat error: %s", chunk.Error.Message)
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" && onDelta != nil {
				onDelta(ch.Delta.Content)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("chat stream: %w", err)
	}
	return nil
}

// --- helpers ---

func expectOK(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, readError(resp))
	}
	return nil
}

// readError extracts a useful message from an error response body.
func readError(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var e struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}
	if json.Unmarshal(b, &e) == nil {
		if e.Error != "" {
			return e.Error
		}
		if e.Detail != "" {
			return e.Detail
		}
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return fmt.Sprintf("status %d", resp.StatusCode)
	}
	return s
}

// drain ensures the body is fully read and closed so connections can be reused.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
}

// compile-time assertion that HTTPClient satisfies Client.
var _ Client = (*HTTPClient)(nil)
