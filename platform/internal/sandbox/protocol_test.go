package sandbox

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvokeRequestMarshal(t *testing.T) {
	req := InvokeRequest{
		ID:     1,
		Method: "invoke",
		Params: InvokeParams{
			Tool:    "image_analysis",
			Payload: map[string]any{"instruction": "extract text"},
			Context: InvokeContext{
				UserID:    "user-123",
				SessionID: "sess-456",
			},
			MediaFiles: []MediaFile{
				{
					MediaID:   "abc123",
					Path:      "/user/media/ab/c1/abc123.png",
					MediaType: "image/png",
					Filename:  "receipt.png",
				},
			},
		},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Round-trip: unmarshal back and verify
	var decoded InvokeRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, 1, decoded.ID)
	assert.Equal(t, "invoke", decoded.Method)
	assert.Equal(t, "image_analysis", decoded.Params.Tool)
	assert.Equal(t, "user-123", decoded.Params.Context.UserID)
	assert.Equal(t, "sess-456", decoded.Params.Context.SessionID)
	assert.Len(t, decoded.Params.MediaFiles, 1)
	assert.Equal(t, "abc123", decoded.Params.MediaFiles[0].MediaID)
	assert.Equal(t, "extract text", decoded.Params.Payload["instruction"])

	// Verify JSON keys
	var raw map[string]any
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)
	assert.Contains(t, raw, "id")
	assert.Contains(t, raw, "method")
	assert.Contains(t, raw, "params")
}

func TestInvokeRequestMarshalNoMediaFiles(t *testing.T) {
	req := InvokeRequest{
		ID:     2,
		Method: "invoke",
		Params: InvokeParams{
			Tool:    "echo",
			Payload: map[string]any{"text": "hello"},
			Context: InvokeContext{UserID: "u1", SessionID: "s1"},
		},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	// media_files should be omitted when empty
	assert.NotContains(t, string(data), "media_files")
}

func TestInvokeResponseUnmarshalSuccess(t *testing.T) {
	raw := `{
		"id": 1,
		"result": {
			"status": "ok",
			"payload": {"output": "hello world"},
			"artifacts": [
				{
					"filename": "chart.png",
					"media_type": "image/png",
					"size_bytes": 45230
				}
			]
		}
	}`

	var resp InvokeResponse
	err := json.Unmarshal([]byte(raw), &resp)
	require.NoError(t, err)

	assert.Equal(t, 1, resp.ID)
	assert.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)
	assert.Equal(t, "ok", resp.Result.Status)
	assert.Equal(t, "hello world", resp.Result.Payload["output"])
	assert.Len(t, resp.Result.Artifacts, 1)
	assert.Equal(t, "chart.png", resp.Result.Artifacts[0].Filename)
	assert.Equal(t, int64(45230), resp.Result.Artifacts[0].SizeBytes)
}

func TestInvokeResponseUnmarshalError(t *testing.T) {
	raw := `{
		"id": 1,
		"error": {
			"code": -1,
			"message": "timeout exceeded"
		}
	}`

	var resp InvokeResponse
	err := json.Unmarshal([]byte(raw), &resp)
	require.NoError(t, err)

	assert.Equal(t, 1, resp.ID)
	assert.Nil(t, resp.Result)
	require.NotNil(t, resp.Error)
	assert.Equal(t, -1, resp.Error.Code)
	assert.Equal(t, "timeout exceeded", resp.Error.Message)
}

func TestInvokeResponseUnmarshalMinimal(t *testing.T) {
	raw := `{"id": 3, "result": {"status": "ok", "payload": {}}}`

	var resp InvokeResponse
	err := json.Unmarshal([]byte(raw), &resp)
	require.NoError(t, err)

	assert.Equal(t, 3, resp.ID)
	require.NotNil(t, resp.Result)
	assert.Equal(t, "ok", resp.Result.Status)
	assert.Empty(t, resp.Result.Artifacts)
}

func TestHostCallbackRequestMarshal(t *testing.T) {
	req := HostCallbackRequest{
		ID:     100,
		Method: "host.readMedia",
		Params: json.RawMessage(`{"media_id":"abc123"}`),
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"method":"host.readMedia"`)
	assert.Contains(t, string(data), `"id":100`)
}

func TestHostCallbackResponseMarshal(t *testing.T) {
	resp := HostCallbackResponse{
		ID:     100,
		Result: json.RawMessage(`{"data":"base64..."}`),
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"id":100`)
	assert.Contains(t, string(data), `"result"`)
}

func TestHostCallbackErrorMarshal(t *testing.T) {
	resp := HostCallbackResponse{
		ID:    100,
		Error: &InvokeError{Code: -1, Message: "access denied"},
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"error"`)
	assert.Contains(t, string(data), `"access denied"`)
}

func TestIsHostCallback(t *testing.T) {
	assert.True(t, IsHostCallback([]byte(`{"method":"host.readMedia","id":100}`)))
	assert.True(t, IsHostCallback([]byte(`{"method":"host.log","id":101}`)))
	assert.False(t, IsHostCallback([]byte(`{"method":"invoke","id":1}`)))
	assert.False(t, IsHostCallback([]byte(`{"id":1,"result":{"status":"ok"}}`)))
	assert.False(t, IsHostCallback([]byte(`invalid json`)))
}
