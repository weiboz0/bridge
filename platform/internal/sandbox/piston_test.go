package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPistonClient_Execute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/execute", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var req PistonExecuteRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "python", req.Language)
		assert.Equal(t, "*", req.Version)
		assert.Len(t, req.Files, 1)
		assert.Equal(t, "print('hello')", req.Files[0].Content)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PistonExecuteResponse{
			Language: "python",
			Version:  "3.10.0",
			Run:      PistonStage{Stdout: "hello\n", Code: 0},
		})
	}))
	defer server.Close()

	client := NewPistonClient(server.URL)
	resp, err := client.Execute(context.Background(), "python", "print('hello')")
	require.NoError(t, err)
	assert.Equal(t, "python", resp.Language)
	assert.Equal(t, "hello\n", resp.Run.Stdout)
	assert.Equal(t, 0, resp.Run.Code)
}

func TestPistonClient_ExecuteWithStdin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req PistonExecuteRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "world", req.Stdin)

		json.NewEncoder(w).Encode(PistonExecuteResponse{
			Language: "python",
			Run:      PistonStage{Stdout: "world\n", Code: 0},
		})
	}))
	defer server.Close()

	client := NewPistonClient(server.URL)
	resp, err := client.ExecuteWithStdin(context.Background(), "python", "print(input())", "world")
	require.NoError(t, err)
	assert.Equal(t, "world\n", resp.Run.Stdout)
}

func TestPistonClient_CompileLanguage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(PistonExecuteResponse{
			Language: "cpp",
			Version:  "10.2.0",
			Compile:  &PistonStage{Stdout: "", Stderr: "", Code: 0},
			Run:      PistonStage{Stdout: "42\n", Code: 0},
		})
	}))
	defer server.Close()

	client := NewPistonClient(server.URL)
	resp, err := client.Execute(context.Background(), "cpp", "int main() { return 0; }")
	require.NoError(t, err)
	assert.NotNil(t, resp.Compile)
	assert.Equal(t, 0, resp.Compile.Code)
}

func TestPistonClient_RuntimeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(PistonExecuteResponse{
			Language: "python",
			Run:      PistonStage{Stderr: "NameError: name 'x' is not defined\n", Code: 1},
		})
	}))
	defer server.Close()

	client := NewPistonClient(server.URL)
	resp, err := client.Execute(context.Background(), "python", "print(x)")
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Run.Code)
	assert.Contains(t, resp.Run.Stderr, "NameError")
}

func TestPistonClient_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"invalid language"}`))
	}))
	defer server.Close()

	client := NewPistonClient(server.URL)
	_, err := client.Execute(context.Background(), "nonexistent", "code")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 400")
}

func TestPistonClient_ServerDown(t *testing.T) {
	client := NewPistonClient("http://localhost:19999")
	_, err := client.Execute(context.Background(), "python", "print(1)")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")
}

func TestPistonClient_ListRuntimes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/runtimes", r.URL.Path)
		json.NewEncoder(w).Encode([]PistonRuntime{
			{Language: "python", Version: "3.10.0", Aliases: []string{"py", "python3"}},
			{Language: "javascript", Version: "18.15.0", Aliases: []string{"js", "node"}},
		})
	}))
	defer server.Close()

	client := NewPistonClient(server.URL)
	runtimes, err := client.ListRuntimes(context.Background())
	require.NoError(t, err)
	assert.Len(t, runtimes, 2)
	assert.Equal(t, "python", runtimes[0].Language)
}

func TestPistonClient_DefaultBaseURL(t *testing.T) {
	client := NewPistonClient("")
	assert.Equal(t, "http://localhost:2000", client.baseURL)
}

func TestPistonExecutor_ImplementsCodeExecutor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(PistonExecuteResponse{
			Language: "python",
			Run:      PistonStage{Stdout: "ok\n", Code: 0},
		})
	}))
	defer server.Close()

	executor := NewPistonExecutor(NewPistonClient(server.URL))
	result, err := executor.Execute(context.Background(), "python", "print('ok')")
	require.NoError(t, err)
	assert.Equal(t, "python", result.Language)
	assert.Equal(t, "ok\n", result.Run.Stdout)
	assert.Equal(t, 0, result.Run.Code)
}
