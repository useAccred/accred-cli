package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const okStream = `: keep-alive

data: {"id":"chatcmpl-gen-1","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-gen-1","model":"m","choices":[{"index":0,"delta":{"content":"Hello, "},"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-gen-1","model":"m","choices":[{"index":0,"delta":{"content":"world"},"finish_reason":null}],"usage":null}

data: {"id":"chatcmpl-gen-1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":null}

data: {"id":"chatcmpl-gen-1","model":"m","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15},"accred":{"credits_charged":"0.000032","provider_cost_usd":"0.000000318","remaining_credits":"99.79"}}

data: [DONE]

`

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c := New("ct_live_test")
	c.BaseURL = server.URL
	c.backoff = func(int) time.Duration { return time.Millisecond }
	return c
}

func stream(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(w, body)
}

func TestChatParsesStream(t *testing.T) {
	var gotAuth, gotKey, gotBody string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotKey = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		stream(w, okStream)
	})

	reply, err := c.Chat(context.Background(), "m", []Message{{Role: "user", Content: "hi"}}, 256, "cli-12345678")
	if err != nil {
		t.Fatal(err)
	}
	want := Reply{ID: "gen-1", Model: "m", Content: "Hello, world", FinishReason: "stop", InputTokens: 12, OutputTokens: 3,
		CreditsCharged: "0.000032", ProviderCostUSD: "0.000000318", RemainingCredits: "99.79"}
	if *reply != want {
		t.Fatalf("reply = %+v, want %+v", *reply, want)
	}
	if gotAuth != "Bearer ct_live_test" || gotKey != "cli-12345678" {
		t.Fatalf("headers: auth=%q key=%q", gotAuth, gotKey)
	}
	for _, part := range []string{`"model":"m"`, `"max_tokens":256`, `"stream":true`, `"include_usage":true`} {
		if !strings.Contains(gotBody, part) {
			t.Fatalf("body %s is missing %s", gotBody, part)
		}
	}
}

func TestChatRetriesWithSameKey(t *testing.T) {
	var keys []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		switch len(keys) {
		case 1:
			http.Error(w, `{"error":{"message":"busy"}}`, http.StatusServiceUnavailable)
		case 2: // a stream that dies mid-reply
			stream(w, `data: {"id":"chatcmpl-x","model":"m","choices":[{"delta":{"content":"par"},"finish_reason":null}]}`+"\n\n")
		default:
			stream(w, okStream)
		}
	})

	reply, err := c.Chat(context.Background(), "m", nil, 0, "cli-12345678")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Content != "Hello, world" {
		t.Fatalf("content = %q", reply.Content)
	}
	if len(keys) != 3 || keys[0] != keys[1] || keys[1] != keys[2] {
		t.Fatalf("idempotency keys = %v", keys)
	}
}

func TestChatDoesNotRetryInsufficientCredit(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		stream(w, `data: {"error":{"message":"Not enough credit","type":"insufficient_quota","code":"insufficient_quota"}}`+"\n\ndata: [DONE]\n\n")
	})

	_, err := c.Chat(context.Background(), "m", nil, 0, "cli-12345678")
	if StatusOf(err) != http.StatusPaymentRequired || err.Error() != "Not enough credit" {
		t.Fatalf("err = %v (status %d)", err, StatusOf(err))
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestChatReportsBadKey(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"The platform API key is invalid or revoked.","type":"authentication_error"}}`)
	})
	_, err := c.Chat(context.Background(), "m", nil, 0, "cli-12345678")
	if StatusOf(err) != http.StatusUnauthorized || !strings.Contains(err.Error(), "invalid or revoked") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatStopsWhenCancelled(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"busy"}`, http.StatusServiceUnavailable)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Chat(ctx, "m", nil, 0, "cli-12345678"); err != context.Canceled {
		t.Fatalf("err = %v", err)
	}
}

func TestModelsKeepsCallableTextModels(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("the catalog must be requested without a key")
		}
		fmt.Fprint(w, `{"models":[
			{"id":"z-text","provider":"p","available":true,"capabilities":["text-generation"],"inputCostUsdPerMillion":"1","outputCostUsdPerMillion":"2"},
			{"id":"a-text","provider":"p","available":true,"capabilities":["text-generation","typed-decisions"]},
			{"id":"off","provider":"p","available":false,"capabilities":["text-generation"]},
			{"id":"image","provider":"p","available":true,"capabilities":["image-generation"]}]}`)
	})
	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "a-text" || models[1].ID != "z-text" {
		t.Fatalf("models = %+v", models)
	}
	if models[1].InputCost == nil || *models[1].InputCost != "1" || models[0].InputCost != nil {
		t.Fatalf("prices = %v %v", models[1].InputCost, models[0].InputCost)
	}
}
