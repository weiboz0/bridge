package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	nextjsURL = envOr("NEXTJS_URL", "http://localhost:3003")
	goURL     = envOr("GO_URL", "http://localhost:8002")
	authToken = os.Getenv("CONTRACT_TEST_TOKEN")
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ContractRequest describes a request to send to both servers.
type ContractRequest struct {
	Method  string
	Path    string
	Body    any
	Headers map[string]string
}

// ContractResult holds responses from both servers.
type ContractResult struct {
	NextStatus int
	GoStatus   int
	NextBody   map[string]any
	GoBody     map[string]any
	NextRaw    string
	GoRaw      string
}

// CompareResponses sends the same request to both servers and returns the results.
func CompareResponses(t *testing.T, req ContractRequest) *ContractResult {
	t.Helper()

	nextResp := sendRequest(t, nextjsURL, req)
	goResp := sendRequest(t, goURL, req)

	return &ContractResult{
		NextStatus: nextResp.statusCode,
		GoStatus:   goResp.statusCode,
		NextBody:   nextResp.body,
		GoBody:     goResp.body,
		NextRaw:    nextResp.raw,
		GoRaw:      goResp.raw,
	}
}

type response struct {
	statusCode int
	body       map[string]any
	raw        string
}

func sendRequest(t *testing.T, baseURL string, req ContractRequest) response {
	t.Helper()

	var bodyReader io.Reader
	if req.Body != nil {
		jsonBytes, err := json.Marshal(req.Body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(jsonBytes)
	}

	httpReq, err := http.NewRequest(req.Method, baseURL+req.Path, bodyReader)
	require.NoError(t, err)

	httpReq.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+authToken)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	require.NoError(t, err, "request to %s%s failed", baseURL, req.Path)
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var body map[string]any
	_ = json.Unmarshal(raw, &body)

	return response{
		statusCode: resp.StatusCode,
		body:       body,
		raw:        string(raw),
	}
}

// AssertSameStatus verifies both servers returned the same HTTP status.
func AssertSameStatus(t *testing.T, result *ContractResult) {
	t.Helper()
	assert.Equal(t, result.NextStatus, result.GoStatus,
		"Status mismatch: Next.js=%d Go=%d\nNext body: %s\nGo body: %s",
		result.NextStatus, result.GoStatus, result.NextRaw, result.GoRaw)
}

// AssertSameKeys verifies both JSON responses have the same top-level keys.
func AssertSameKeys(t *testing.T, result *ContractResult) {
	t.Helper()
	if result.NextBody == nil || result.GoBody == nil {
		return
	}
	for key := range result.NextBody {
		assert.Contains(t, result.GoBody, key,
			"Go response missing key %q present in Next.js response", key)
	}
}

// AssertFieldEqual verifies a specific field has the same value in both responses.
func AssertFieldEqual(t *testing.T, result *ContractResult, field string) {
	t.Helper()
	if result.NextBody == nil || result.GoBody == nil {
		return
	}
	assert.Equal(t, result.NextBody[field], result.GoBody[field],
		"Field %q differs: Next.js=%v Go=%v", field, result.NextBody[field], result.GoBody[field])
}

// SkipIfNoServers skips the test if either server is unreachable or token is missing.
func SkipIfNoServers(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}

	for _, url := range []string{nextjsURL, goURL} {
		resp, err := client.Get(url + "/healthz")
		if err != nil {
			t.Skipf("Server %s not reachable: %v", url, err)
		}
		resp.Body.Close()
	}

	if authToken == "" {
		t.Skip("CONTRACT_TEST_TOKEN not set")
	}
}

// SkipIfGoDown skips the test if the Go server is unreachable.
func SkipIfGoDown(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(goURL + "/healthz")
	if err != nil {
		t.Skipf("Go server not reachable: %v", err)
	}
	resp.Body.Close()
	if authToken == "" {
		t.Skip("CONTRACT_TEST_TOKEN not set")
	}
}

// PrintDiff logs the differences between two responses for debugging.
func PrintDiff(t *testing.T, result *ContractResult) {
	t.Helper()
	if result.NextStatus != result.GoStatus {
		t.Logf("Status: Next=%d Go=%d", result.NextStatus, result.GoStatus)
	}
	t.Logf("Next.js body: %s", result.NextRaw)
	t.Logf("Go body: %s", result.GoRaw)
}
