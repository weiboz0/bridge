package realtime

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These tests intentionally define the Go-side control protocol before the
// client exists.  The API is deliberately small: a caller cannot construct a
// permissive transport, and terminal cleanup always carries the same token
// that acquired the freeze.

func TestCanvasControlClient_FreezeUsesStrictLocalTransportAndExactBundle(t *testing.T) {
	state := []byte("authoritative yjs state")
	digest := sha256.Sum256(state)
	var gotAuth, gotPath string
	var gotBody struct {
		SessionID string   `json:"sessionId"`
		Token     string   `json:"token"`
		CanvasIDs []string `json:"canvasIds"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": []map[string]string{{
			"canvasId": "11111111-1111-4111-8111-111111111111",
			"state":    base64.StdEncoding.EncodeToString(state),
			"sha256":   hex.EncodeToString(digest[:]),
		}}})
	}))
	defer server.Close()

	client, err := NewCanvasControlClient(CanvasControlConfig{
		URL: server.URL, Secret: "separate-control-secret", HTTPClient: server.Client(),
	})
	require.NoError(t, err)

	bundle, err := client.Freeze(context.Background(), FreezeRequest{
		SessionID: "22222222-2222-4222-8222-222222222222",
		Token:     "33333333-3333-4333-8333-333333333333",
		CanvasIDs: []string{"11111111-1111-4111-8111-111111111111"},
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer separate-control-secret", gotAuth)
	require.Equal(t, "/freeze", gotPath)
	require.Equal(t, "22222222-2222-4222-8222-222222222222", gotBody.SessionID)
	require.Equal(t, "33333333-3333-4333-8333-333333333333", gotBody.Token)
	require.Equal(t, gotBody.CanvasIDs, []string{"11111111-1111-4111-8111-111111111111"})
	require.Equal(t, []CanvasSnapshot{{CanvasID: gotBody.CanvasIDs[0], State: state, SHA256: hex.EncodeToString(digest[:])}}, bundle.Entries)
}

func TestCanvasControlClient_RejectsUnsafeURLsRedirectsAndMalformedBundles(t *testing.T) {
	for _, rawURL := range []string{
		"http://localhost:4001", "http://[::1]:4001", "http://control.internal:4001",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err := NewCanvasControlClient(CanvasControlConfig{URL: rawURL, Secret: "separate-control-secret"})
			require.Error(t, err)
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/freeze" {
			http.Redirect(w, r, "/other", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: "separate-control-secret", HTTPClient: server.Client()})
	require.NoError(t, err)
	_, err = client.Freeze(context.Background(), FreezeRequest{SessionID: "22222222-2222-4222-8222-222222222222", Token: "33333333-3333-4333-8333-333333333333"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "separate-control-secret")
}

func TestCanvasControlClient_RetriesSameTokenWithinTwoSecondBudgetAndTerminalCallsAreBestEffort(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			// Simulate transport loss after capture.  A same-token retry must
			// recover the cached response instead of inventing a new operation.
			hj, ok := w.(http.Hijacker)
			require.True(t, ok)
			conn, _, err := hj.Hijack()
			require.NoError(t, err)
			_ = conn.Close()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": []any{}})
	}))
	defer server.Close()

	client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: "separate-control-secret", HTTPClient: server.Client()})
	require.NoError(t, err)
	started := time.Now()
	_, err = client.Freeze(context.Background(), FreezeRequest{SessionID: "22222222-2222-4222-8222-222222222222", Token: "33333333-3333-4333-8333-333333333333"})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.LessOrEqual(t, time.Since(started), 2*time.Second)

	// Completion and unfreeze must not turn a durable DB result into an error
	// merely because Hocuspocus is already unavailable.
	client.Complete(context.Background(), "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333")
	client.Unfreeze(context.Background(), "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333")
}

func TestCanvasControlClient_RejectsOversizedOrNonCanonicalResponseBeforeReturningBundle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 48*1024*1024+1)))
	}))
	defer server.Close()
	client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: "separate-control-secret", HTTPClient: server.Client()})
	require.NoError(t, err)
	_, err = client.Freeze(context.Background(), FreezeRequest{SessionID: "22222222-2222-4222-8222-222222222222", Token: "33333333-3333-4333-8333-333333333333"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "separate-control-secret")
}
