// Package api talks to the Accred HTTP API.
package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const DefaultBaseURL = "https://accred.sh"

type Client struct {
	BaseURL    string
	APIKey     string
	HTTP       *http.Client
	MaxRetries int
	// backoff is replaced in tests.
	backoff func(attempt int) time.Duration
}

func New(apiKey string) *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		APIKey:     apiKey,
		HTTP:       &http.Client{},
		MaxRetries: 2,
		backoff: func(attempt int) time.Duration {
			return min(500*time.Millisecond<<attempt, 8*time.Second)
		},
	}
}

// Error is a failure reported by the API. Status is 0 when no response arrived.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// StatusOf returns the HTTP status behind err, or 0.
func StatusOf(err error) int {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Model struct {
	Provider        string   `json:"provider"`
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	InputCost       *string  `json:"inputCostUsdPerMillion"`
	OutputCost      *string  `json:"outputCostUsdPerMillion"`
	Capabilities    []string `json:"capabilities"`
	Available       bool     `json:"available"`
	MaxOutputTokens *int     `json:"maxOutputTokens"`
}

type Reply struct {
	ID               string
	Model            string
	Content          string
	FinishReason     string
	InputTokens      int
	OutputTokens     int
	CreditsCharged   string
	ProviderCostUSD  string
	RemainingCredits string // empty when the API could not read it
}

// NewIdempotencyKey returns a fresh key for one logical chat call.
func NewIdempotencyKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "cli-" + hex.EncodeToString(b)
}

// Models returns the text models that can be called right now, sorted by id.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/customer/models", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &Error{Message: "could not reach Accred: " + err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errorFromResponse(res)
	}
	var body struct {
		Models []Model `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, &Error{Message: "unreadable model catalog: " + err.Error()}
	}
	models := body.Models[:0]
	for _, m := range body.Models {
		if m.Available && hasCapability(m, "text-generation") {
			models = append(models, m)
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func hasCapability(m Model, capability string) bool {
	for _, c := range m.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

// ValidateKey reports whether the API key is accepted.
func (c *Client) ValidateKey(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/openai/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return &Error{Message: "could not reach Accred: " + err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errorFromResponse(res)
	}
	return nil
}

// Chat runs one completion. It retries rate limits, gateway errors and dropped
// connections with the same idempotency key, so a call is never charged twice.
func (c *Client) Chat(ctx context.Context, model string, messages []Message, maxTokens int, idempotencyKey string) (*Reply, error) {
	payload := map[string]any{
		"model":    model,
		"messages": messages,
		// The gateway sends keep-alives on a stream, which protects long calls from idle timeouts.
		"stream":         true,
		"stream_options": map[string]bool{"include_usage": true},
	}
	if maxTokens > 0 {
		payload["max_tokens"] = maxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	for attempt := 0; ; attempt++ {
		reply, err := c.chatOnce(ctx, body, idempotencyKey)
		if err == nil {
			return reply, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt >= c.MaxRetries || !retryable(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.backoff(attempt)):
		}
	}
}

func retryable(err error) bool {
	switch StatusOf(err) {
	case 0, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

type streamEvent struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Accred *struct {
		CreditsCharged   string  `json:"credits_charged"`
		ProviderCostUSD  string  `json:"provider_cost_usd"`
		RemainingCredits *string `json:"remaining_credits"`
	} `json:"accred"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (c *Client) chatOnce(ctx context.Context, body []byte, idempotencyKey string) (*Reply, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/openai/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &Error{Message: "could not reach Accred: " + err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errorFromResponse(res)
	}

	reply := &Reply{}
	var content strings.Builder
	finished := false
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok {
			continue // blank separators and ": keep-alive" comments
		}
		if data == "[DONE]" {
			break
		}
		var event streamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, &Error{Message: "unreadable reply from Accred: " + err.Error()}
		}
		if event.Error != nil {
			return nil, &Error{Status: statusForErrorType(event.Error.Type), Message: event.Error.Message}
		}
		if event.ID != "" {
			reply.ID = strings.TrimPrefix(event.ID, "chatcmpl-")
			reply.Model = event.Model
		}
		for _, choice := range event.Choices {
			content.WriteString(choice.Delta.Content)
			if choice.FinishReason != nil {
				reply.FinishReason = *choice.FinishReason
				finished = true
			}
		}
		if event.Usage != nil {
			reply.InputTokens = event.Usage.PromptTokens
			reply.OutputTokens = event.Usage.CompletionTokens
		}
		if event.Accred != nil {
			reply.CreditsCharged = event.Accred.CreditsCharged
			reply.ProviderCostUSD = event.Accred.ProviderCostUSD
			if event.Accred.RemainingCredits != nil {
				reply.RemainingCredits = *event.Accred.RemainingCredits
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, &Error{Message: "connection to Accred dropped: " + err.Error()}
	}
	if !finished {
		return nil, &Error{Message: "connection to Accred closed before the reply finished"}
	}
	reply.Content = content.String()
	return reply, nil
}

// Errors inside a stream arrive after the 200 header, so the status is recovered from the error type.
func statusForErrorType(errorType string) int {
	switch errorType {
	case "invalid_request_error":
		return http.StatusBadRequest
	case "authentication_error":
		return http.StatusUnauthorized
	case "insufficient_quota":
		return http.StatusPaymentRequired
	case "rate_limit_error":
		return http.StatusTooManyRequests
	}
	return http.StatusServiceUnavailable
}

func errorFromResponse(res *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
	// The OpenAI-style routes nest the message; the native routes return it as a string.
	var nested struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	var flat struct {
		Error string `json:"error"`
	}
	message := ""
	if json.Unmarshal(raw, &nested) == nil && nested.Error.Message != "" {
		message = nested.Error.Message
	} else if json.Unmarshal(raw, &flat) == nil && flat.Error != "" {
		message = flat.Error
	} else {
		message = fmt.Sprintf("Accred request failed with status %d", res.StatusCode)
	}
	return &Error{Status: res.StatusCode, Message: message}
}
