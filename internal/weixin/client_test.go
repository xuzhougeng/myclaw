package weixin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newTestClient(t *testing.T, got *SendMessageRequest) *Client {
	t.Helper()

	client := NewClient("http://unit.test", "secret-token")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/ilink/bot/sendmessage" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.Header.Get("AuthorizationType") != "ilink_bot_token" {
				t.Fatalf("unexpected auth type: %q", r.Header.Get("AuthorizationType"))
			}
			if r.Header.Get("Authorization") != "Bearer secret-token" {
				t.Fatalf("unexpected auth header: %q", r.Header.Get("Authorization"))
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if err := json.Unmarshal(body, got); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{"ret":0}`)),
			}, nil
		}),
	}
	return client
}
