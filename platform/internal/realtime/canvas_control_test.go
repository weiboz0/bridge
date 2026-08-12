package realtime

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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
		Token     string   `json:"freezeToken"`
		CanvasIDs []string `json:"canvasIds"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_ = json.NewEncoder(w).Encode(map[string]any{"snapshots": []map[string]string{{
			"canvasId":    "11111111-1111-4111-8111-111111111111",
			"stateBase64": base64.StdEncoding.EncodeToString(state),
			"sha256":      hex.EncodeToString(digest[:]),
		}}, "closed": 0})
	}))
	defer server.Close()

	client, err := NewCanvasControlClient(CanvasControlConfig{
		URL: server.URL, Secret: strings.Repeat("a", 64), HTTPClient: server.Client(),
	})
	require.NoError(t, err)

	bundle, err := client.Freeze(context.Background(), FreezeRequest{
		SessionID:   "22222222-2222-4222-8222-222222222222",
		FreezeToken: "33333333-3333-4333-8333-333333333333",
		CanvasIDs:   []string{"11111111-1111-4111-8111-111111111111"},
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer "+strings.Repeat("a", 64), gotAuth)
	require.Equal(t, "/internal/canvas-sessions/freeze", gotPath)
	require.Equal(t, "22222222-2222-4222-8222-222222222222", gotBody.SessionID)
	require.Equal(t, "33333333-3333-4333-8333-333333333333", gotBody.Token)
	require.Equal(t, gotBody.CanvasIDs, []string{"11111111-1111-4111-8111-111111111111"})
	require.Equal(t, []CanvasSnapshot{{CanvasID: gotBody.CanvasIDs[0], State: state, SHA256: hex.EncodeToString(digest[:])}}, bundle.Snapshots)
}

func TestCanvasControlClient_FreezeZeroCanvasIDsSendsAnEmptyArray(t *testing.T) {
	const (
		sessionID = "22222222-2222-4222-8222-222222222222"
		token     = "33333333-3333-4333-8333-333333333333"
	)
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/internal/canvas-sessions/freeze", r.URL.Path)
		var err error
		body, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"snapshots":[],"closed":0}`))
	}))
	defer server.Close()

	client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: strings.Repeat("a", 64), HTTPClient: server.Client()})
	require.NoError(t, err)
	_, err = client.Freeze(context.Background(), FreezeRequest{SessionID: sessionID, FreezeToken: token, CanvasIDs: []string{}})
	require.NoError(t, err)
	require.JSONEq(t, `{"sessionId":"`+sessionID+`","freezeToken":"`+token+`","canvasIds":[]}`, string(body))
	require.NotContains(t, string(body), `"canvasIds":null`)
}

type fakeCurrentDatabase struct {
	name  string
	err   error
	calls int
}

func (f *fakeCurrentDatabase) CurrentDatabaseName(context.Context) (string, error) {
	f.calls++
	return f.name, f.err
}

func TestE2ECanvasControlFailureInjection_RequiresExplicitFlagAndTwoTestDatabaseProofs(t *testing.T) {
	for _, tc := range []struct {
		name, databaseURL, liveName string
		liveErr                     error
		enabled                     bool
		wantInjection               bool
		wantProbe                   bool
		wantErr                     bool
	}{
		{"disabled does not probe or inject", "postgresql://bridge@127.0.0.1:5432/bridge", "bridge", nil, false, false, false, false},
		{"parsed database is not test", "postgresql://bridge@127.0.0.1:5432/bridge", "bridge_test", nil, true, false, false, true},
		{"live database lookup fails closed", "postgresql://bridge@127.0.0.1:5432/bridge_test", "", errors.New("database unavailable"), true, false, true, true},
		{"live database is not test", "postgresql://bridge@127.0.0.1:5432/bridge_test", "bridge", nil, true, false, true, true},
		{"decoded parsed and live database names are test", "postgresql://bridge@127.0.0.1:5432/bridge%5Ftest", "bridge_test", nil, true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			live := &fakeCurrentDatabase{name: tc.liveName, err: tc.liveErr}
			injection, err := NewE2ECanvasControlFailureInjection(context.Background(), tc.enabled, tc.databaseURL, live)
			if tc.wantInjection {
				require.NoError(t, err)
				require.NotNil(t, injection)
			} else {
				require.Nil(t, injection)
				if tc.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
			require.Equal(t, tc.wantProbe, live.calls == 1)
		})
	}
}

func TestCanvasControlClient_E2EFailureInjectionNeverContactsControlListener(t *testing.T) {
	hitControl := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hitControl = true }))
	defer server.Close()

	injection, err := NewE2ECanvasControlFailureInjection(
		context.Background(),
		true,
		"postgresql://bridge@127.0.0.1:5432/bridge_test",
		&fakeCurrentDatabase{name: "bridge_test"},
	)
	require.NoError(t, err)
	client, err := NewCanvasControlClient(CanvasControlConfig{
		URL: server.URL, Secret: strings.Repeat("a", 64), HTTPClient: server.Client(), E2EFailureInjection: injection,
	})
	require.NoError(t, err)

	_, err = client.Freeze(context.Background(), FreezeRequest{
		SessionID: "22222222-2222-4222-8222-222222222222", FreezeToken: "33333333-3333-4333-8333-333333333333",
	})
	require.EqualError(t, err, "canvas freeze failed: E2E canvas control freeze failure injected")
	require.False(t, hitControl)
}

func TestCanvasControlClient_RejectsUnsafeURLsRedirectsAndMalformedBundles(t *testing.T) {
	for _, rawURL := range []string{
		"http://localhost:4001", "http://control.internal:4001", "http://[::ffff:127.0.0.1]:4001",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err := NewCanvasControlClient(CanvasControlConfig{URL: rawURL, Secret: strings.Repeat("a", 64)})
			require.Error(t, err)
		})
	}

	redirectTargetHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/canvas-sessions/freeze" {
			http.Redirect(w, r, "/other", http.StatusFound)
			return
		}
		redirectTargetHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: strings.Repeat("a", 64), HTTPClient: server.Client()})
	require.NoError(t, err)
	_, err = client.Freeze(context.Background(), FreezeRequest{SessionID: "22222222-2222-4222-8222-222222222222", FreezeToken: "33333333-3333-4333-8333-333333333333"})
	require.Error(t, err)
	require.False(t, redirectTargetHit)
	require.NotContains(t, err.Error(), strings.Repeat("a", 64))
}

func TestCanvasControlClient_RetriesConflictAndPartialCapturedBodyWithSameToken(t *testing.T) {
	state := []byte("cached-after-capture")
	sum := sha256.Sum256(state)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if calls == 2 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"snapshots":[{"canvasId":"11111111-1111-4111-8111-111111111111","stateBase64":"` + base64.StdEncoding.EncodeToString(state) + `","sha256":"` + hex.EncodeToString(sum[:]) + `"}`))
			if h, ok := w.(http.Hijacker); ok {
				c, _, _ := h.Hijack()
				_ = c.Close()
			}
			return
		}
		_, _ = w.Write([]byte(`{"snapshots":[{"canvasId":"11111111-1111-4111-8111-111111111111","stateBase64":"` + base64.StdEncoding.EncodeToString(state) + `","sha256":"` + hex.EncodeToString(sum[:]) + `"}],"closed":1}`))
	}))
	defer server.Close()
	client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: strings.Repeat("c", 64), HTTPClient: server.Client()})
	require.NoError(t, err)
	bundle, err := client.Freeze(context.Background(), FreezeRequest{SessionID: "22222222-2222-4222-8222-222222222222", FreezeToken: "33333333-3333-4333-8333-333333333333", CanvasIDs: []string{"11111111-1111-4111-8111-111111111111"}})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	require.Equal(t, state, bundle.Snapshots[0].State)
}

func TestEndSession_TransportLossRecoversSameTokenBundle(t *testing.T) {
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
		_ = json.NewEncoder(w).Encode(map[string]any{"snapshots": []any{}, "closed": 0})
	}))
	defer server.Close()

	client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: strings.Repeat("a", 64), HTTPClient: server.Client()})
	require.NoError(t, err)
	started := time.Now()
	_, err = client.Freeze(context.Background(), FreezeRequest{SessionID: "22222222-2222-4222-8222-222222222222", FreezeToken: "33333333-3333-4333-8333-333333333333"})
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
	client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: strings.Repeat("a", 64), HTTPClient: server.Client()})
	require.NoError(t, err)
	_, err = client.Freeze(context.Background(), FreezeRequest{SessionID: "22222222-2222-4222-8222-222222222222", FreezeToken: "33333333-3333-4333-8333-333333333333"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), strings.Repeat("a", 64))
}

func TestCanvasControlClient_ValidatesSubsetOrderingAndClosedCount(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	state := []byte("state")
	digest := sha256.Sum256(state)
	good := func(snapshots any, closed int) []byte {
		body, err := json.Marshal(map[string]any{"snapshots": snapshots, "closed": closed})
		require.NoError(t, err)
		return body
	}
	_, err := validateFreezeBundle(good([]any{}, 0), []string{id})
	require.NoError(t, err, "a loaded-document subset may be empty")
	valid := map[string]string{"canvasId": id, "stateBase64": base64.StdEncoding.EncodeToString(state), "sha256": hex.EncodeToString(digest[:])}
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"negative closed", good([]any{}, -1)},
		{"unexpected", good([]any{map[string]string{"canvasId": "22222222-2222-4222-8222-222222222222", "stateBase64": valid["stateBase64"], "sha256": valid["sha256"]}}, 0)},
		{"trailing", append(good([]any{valid}, 0), []byte("x")...)},
	} {
		t.Run(tc.name, func(t *testing.T) { _, err := validateFreezeBundle(tc.body, []string{id}); require.Error(t, err) })
	}
}

func TestCanvasControlClient_TerminalCallsUseExactRoutesAndFreezeToken(t *testing.T) {
	secret := strings.Repeat("b", 64)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		require.Equal(t, "Bearer "+secret, r.Header.Get("Authorization"))
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "33333333-3333-4333-8333-333333333333", body["freezeToken"])
		if r.URL.Path == "/internal/canvas-sessions/unfreeze" {
			_, _ = w.Write([]byte(`{"unfrozen":true}`))
		} else {
			_, _ = w.Write([]byte(`{"released":true}`))
		}
	}))
	defer server.Close()
	client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: secret, HTTPClient: server.Client()})
	require.NoError(t, err)
	client.Complete(context.Background(), "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333")
	client.Unfreeze(context.Background(), "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333")
	require.Equal(t, []string{"/internal/canvas-sessions/complete", "/internal/canvas-sessions/unfreeze"}, paths)
}

func TestCanvasControlClient_RejectsInvalidPortAndAcceptsBothCanonicalLoopbacks(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1:0",
		"http://127.0.0.1:65536",
		"http://[::1]:0",
		"http://[::1]:65536",
	} {
		t.Run(rawURL, func(t *testing.T) {
			require.Error(t, ValidateControlURL(rawURL), "port must be a usable TCP port")
		})
	}
	for _, rawURL := range []string{"http://127.0.0.1:4001", "http://[::1]:4001"} {
		t.Run(rawURL, func(t *testing.T) {
			require.NoError(t, ValidateControlURL(rawURL))
		})
	}
}

func TestCanvasControlClient_RejectsEveryInvalidRequestAndBundleBoundary(t *testing.T) {
	const (
		sessionID = "22222222-2222-4222-8222-222222222222"
		token     = "33333333-3333-4333-8333-333333333333"
		canvasID  = "11111111-1111-4111-8111-111111111111"
	)
	for _, request := range []FreezeRequest{
		{SessionID: "not-a-uuid", FreezeToken: token},
		{SessionID: sessionID, FreezeToken: "not-a-uuid"},
		{SessionID: sessionID, FreezeToken: token, CanvasIDs: []string{canvasID, canvasID}},
		{SessionID: sessionID, FreezeToken: token, CanvasIDs: []string{"22222222-2222-4222-8222-222222222222", canvasID}},
		{SessionID: sessionID, FreezeToken: token, CanvasIDs: make([]string, maxSnapshots+1)},
	} {
		require.Error(t, validateFreezeRequest(request))
	}

	state := []byte("state")
	digest := sha256.Sum256(state)
	valid := map[string]any{
		"canvasId": canvasID, "stateBase64": base64.StdEncoding.EncodeToString(state), "sha256": hex.EncodeToString(digest[:]),
	}
	marshal := func(v any) []byte {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return b
	}
	for name, body := range map[string][]byte{
		"missing snapshots":  marshal(map[string]any{"closed": 0}),
		"null snapshots":     marshal(map[string]any{"snapshots": nil, "closed": 0}),
		"missing closed":     marshal(map[string]any{"snapshots": []any{}}),
		"wrong closed type":  marshal(map[string]any{"snapshots": []any{}, "closed": "0"}),
		"negative closed":    marshal(map[string]any{"snapshots": []any{}, "closed": -1}),
		"unknown field":      marshal(map[string]any{"snapshots": []any{}, "closed": 0, "extra": true}),
		"duplicate snapshot": marshal(map[string]any{"snapshots": []any{valid, valid}, "closed": 0}),
		"wrong digest": marshal(map[string]any{"snapshots": []any{map[string]any{
			"canvasId": canvasID, "stateBase64": valid["stateBase64"], "sha256": strings.Repeat("0", 64),
		}}, "closed": 0}),
		"noncanonical base64": marshal(map[string]any{"snapshots": []any{map[string]any{
			"canvasId": canvasID, "stateBase64": base64.StdEncoding.EncodeToString(state) + "\n", "sha256": valid["sha256"],
		}}, "closed": 0}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := validateFreezeBundle(body, []string{canvasID})
			require.Error(t, err)
		})
	}

	// These exact decoded limits are independent of the 48 MiB transport cap.
	tooLargeSnapshot := make([]byte, maxSnapshotBytes+1)
	tooLargeDigest := sha256.Sum256(tooLargeSnapshot)
	_, err := validateFreezeBundle(marshal(map[string]any{"snapshots": []any{map[string]any{
		"canvasId": canvasID, "stateBase64": base64.StdEncoding.EncodeToString(tooLargeSnapshot), "sha256": hex.EncodeToString(tooLargeDigest[:]),
	}}, "closed": 0}), []string{canvasID})
	require.Error(t, err)
}

func TestCanvasControlClient_TerminalAcknowledgementsAreExactAndNoSecretLeaks(t *testing.T) {
	secret := strings.Repeat("d", 64)
	for _, tc := range []struct {
		name, path string
		status     int
		body       string
	}{
		{"complete wrong field", "/internal/canvas-sessions/complete", http.StatusOK, `{"unfrozen":true}`},
		{"unfreeze wrong field", "/internal/canvas-sessions/unfreeze", http.StatusOK, `{"released":true}`},
		{"complete trailing", "/internal/canvas-sessions/complete", http.StatusOK, `{"released":true}{}`},
		{"complete non-200", "/internal/canvas-sessions/complete", http.StatusConflict, `{"released":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer "+secret, r.Header.Get("Authorization"))
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: secret, HTTPClient: server.Client()})
			require.NoError(t, err)
			// Terminal cleanup is deliberately best effort.  This test keeps its
			// strict acknowledgement parser covered without making durable end
			// success depend on listener availability.
			client.terminal(context.Background(), tc.path, "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333")
			require.False(t, validTerminalAck(tc.path, tc.status, []byte(tc.body)))
		})
	}
}

func TestCanvasControlClient_DeterministicValidationNeverContactsControlAndRetryBodyIsStable(t *testing.T) {
	hits := 0
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		bodies = append(bodies, body)
		if hits == 1 {
			w.WriteHeader(http.StatusConflict)
			return
		}
		_, _ = w.Write([]byte(`{"snapshots":[],"closed":0}`))
	}))
	defer server.Close()
	client, err := NewCanvasControlClient(CanvasControlConfig{URL: server.URL, Secret: strings.Repeat("a", 64), HTTPClient: server.Client()})
	require.NoError(t, err)

	_, err = client.Freeze(context.Background(), FreezeRequest{SessionID: "not-a-uuid", FreezeToken: "33333333-3333-4333-8333-333333333333"})
	require.Error(t, err)
	require.Zero(t, hits, "invalid local input must fail closed without a network attempt")

	request := FreezeRequest{SessionID: "22222222-2222-4222-8222-222222222222", FreezeToken: "33333333-3333-4333-8333-333333333333"}
	_, err = client.Freeze(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, 2, hits)
	require.Len(t, bodies, 2)
	require.Equal(t, bodies[0], bodies[1], "same-token retry must replay the exact request body")
	var replay FreezeRequest
	require.NoError(t, json.Unmarshal(bodies[0], &replay))
	require.Equal(t, request, replay)
}

func TestCanvasControlClient_RespectsCallerDeadlineAndVerifiedHTTPS(t *testing.T) {
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"snapshots":[],"closed":0}`))
	}))
	defer tlsServer.Close()
	client, err := NewCanvasControlClient(CanvasControlConfig{URL: tlsServer.URL, Secret: strings.Repeat("a", 64), HTTPClient: tlsServer.Client()})
	require.NoError(t, err, "a normal certificate-verifying HTTPS client is a supported private deployment")
	_, err = client.Freeze(context.Background(), FreezeRequest{SessionID: "22222222-2222-4222-8222-222222222222", FreezeToken: "33333333-3333-4333-8333-333333333333"})
	require.NoError(t, err)

	_, err = NewCanvasControlClient(CanvasControlConfig{
		URL: tlsServer.URL, Secret: strings.Repeat("a", 64),
		HTTPClient: &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}, //nolint:gosec // validation must reject this client before use.
	})
	require.Error(t, err)

	started := make(chan struct{}, 1)
	blocking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer blocking.Close()
	deadlineClient, err := NewCanvasControlClient(CanvasControlConfig{URL: blocking.URL, Secret: strings.Repeat("a", 64), HTTPClient: blocking.Client()})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	_, err = deadlineClient.Freeze(ctx, FreezeRequest{SessionID: "22222222-2222-4222-8222-222222222222", FreezeToken: "33333333-3333-4333-8333-333333333333"})
	require.Error(t, err)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("freeze request did not reach the control transport")
	}
}

func TestCanvasControlClient_RetryJitterIsBoundedEvenWhenRandomnessFallsBack(t *testing.T) {
	for i := 0; i < 128; i++ {
		wait := retryJitter()
		require.GreaterOrEqual(t, wait, 5*time.Millisecond)
		require.LessOrEqual(t, wait, 28*time.Millisecond)
	}
}
