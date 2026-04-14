package sandbox

import "encoding/json"

// InvokeRequest is sent from Go to a subprocess via stdin (JSON-RPC style).
type InvokeRequest struct {
	ID     int          `json:"id"`
	Method string       `json:"method"` // "invoke"
	Params InvokeParams `json:"params"`
}

// InvokeParams carries the tool invocation details.
type InvokeParams struct {
	Tool       string         `json:"tool"`
	Payload    map[string]any `json:"payload"`
	Context    InvokeContext  `json:"context"`
	MediaFiles []MediaFile    `json:"media_files,omitempty"`
}

// InvokeContext provides identity information to the sandboxed process.
type InvokeContext struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
}

// MediaFile describes a media file available inside the jail.
type MediaFile struct {
	MediaID   string `json:"media_id"`
	Path      string `json:"path"`       // path inside jail (e.g. /user/media/ab/c1/abc123.png)
	MediaType string `json:"media_type"` // MIME type
	Filename  string `json:"filename"`   // original filename
}

// InvokeResponse is read from subprocess stdout (JSON-RPC style).
type InvokeResponse struct {
	ID     int           `json:"id"`
	Result *InvokeResult `json:"result,omitempty"`
	Error  *InvokeError  `json:"error,omitempty"`
}

// InvokeResult represents a successful tool execution.
type InvokeResult struct {
	Status    string         `json:"status"` // "ok" or "error"
	Payload   map[string]any `json:"payload"`
	Artifacts []Artifact     `json:"artifacts,omitempty"`
}

// Artifact describes a file produced by the sandboxed skill.
type Artifact struct {
	Filename  string `json:"filename"`
	MediaType string `json:"media_type"`
	MediaID   string `json:"media_id,omitempty"` // set by Go after media ingestion
	SizeBytes int64  `json:"size_bytes"`
}

// InvokeError represents a tool execution failure.
type InvokeError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// HostCallbackRequest is sent from Deno to Go requesting a host function.
type HostCallbackRequest struct {
	ID     int             `json:"id"`
	Method string          `json:"method"` // "host.readMedia", "host.searchWeb", etc.
	Params json.RawMessage `json:"params"`
}

// HostCallbackResponse is sent from Go to Deno with the callback result.
type HostCallbackResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *InvokeError    `json:"error,omitempty"`
}

// IsHostCallback checks if a raw JSON message is a host callback (method starts with "host.").
func IsHostCallback(data []byte) bool {
	var peek struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return false
	}
	return len(peek.Method) > 5 && peek.Method[:5] == "host."
}
