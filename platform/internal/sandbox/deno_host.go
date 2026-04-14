package sandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
)

// hostMethodAccess maps host methods to required access permissions.
var hostMethodAccess = map[string]string{
	"host.log":            "", // always allowed
	"host.getSkillConfig": "", // always allowed
	"host.readMedia":      "media",
	"host.getMediaInfo":   "media",
	"host.kvGet":          "storage",
	"host.kvSet":          "storage",
	"host.kvDelete":       "storage",
	"host.searchWeb":      "search",
	"host.fetch":          "network",
	"host.chatComplete":   "llm",
}

// MediaProvider is the interface for media host callbacks (breaks import cycle with media package).
type MediaProvider interface {
	Get(ctx context.Context, mediaID string) (map[string]any, error)
	GetFileBytes(ctx context.Context, mediaID string) ([]byte, error)
}

// SearchProvider is the interface for web search host callback.
type SearchProvider interface {
	Search(query string) ([]map[string]any, error)
}

// LLMProvider is the interface for chat completion host callback.
type LLMProvider interface {
	ChatComplete(messages []map[string]any, model string) (string, error)
}

// HostDispatcher handles host.* callback requests from Deno skills.
type HostDispatcher struct {
	media  MediaProvider
	search SearchProvider
	llm    LLMProvider
	access map[string]bool
}

// NewHostDispatcher creates a HostDispatcher with the given providers and access map.
func NewHostDispatcher(mediaStore MediaProvider, search SearchProvider, llm LLMProvider, access map[string]bool) *HostDispatcher {
	return &HostDispatcher{media: mediaStore, search: search, llm: llm, access: access}
}

// Dispatch routes a host callback to the appropriate handler.
func (h *HostDispatcher) Dispatch(ctx context.Context, method string, params json.RawMessage) (any, error) {
	required, known := hostMethodAccess[method]
	if !known {
		return nil, fmt.Errorf("unknown host method: %s", method)
	}
	if required != "" && !h.access[required] {
		return nil, fmt.Errorf("skill does not have '%s' access (required by %s)", required, method)
	}

	switch method {
	case "host.log":
		var p struct {
			Level   string `json:"level"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("parse host.log params: %w", err)
		}
		slog.Info("skill.log", "level", p.Level, "message", p.Message)
		return map[string]string{"status": "ok"}, nil

	case "host.getSkillConfig":
		return map[string]string{}, nil

	case "host.readMedia":
		var p struct {
			MediaID string `json:"media_id"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("parse host.readMedia params: %w", err)
		}
		if h.media == nil {
			return nil, fmt.Errorf("media store not available")
		}
		data, err := h.media.GetFileBytes(ctx, p.MediaID)
		if err != nil {
			return nil, err
		}
		return map[string]string{"data": base64.StdEncoding.EncodeToString(data)}, nil

	case "host.getMediaInfo":
		var p struct {
			MediaID string `json:"media_id"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("parse host.getMediaInfo params: %w", err)
		}
		if h.media == nil {
			return nil, fmt.Errorf("media store not available")
		}
		info, err := h.media.Get(ctx, p.MediaID)
		if err != nil {
			return nil, err
		}
		return info, nil

	case "host.kvGet":
		return map[string]any{"value": nil}, nil // placeholder — backed by DB in future

	case "host.kvSet":
		return map[string]string{"status": "ok"}, nil // placeholder

	case "host.kvDelete":
		return map[string]string{"status": "ok"}, nil // placeholder

	case "host.searchWeb":
		var p struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("parse host.searchWeb params: %w", err)
		}
		if h.search == nil {
			return nil, fmt.Errorf("search provider not available")
		}
		return h.search.Search(p.Query)

	case "host.fetch":
		return nil, fmt.Errorf("host.fetch is not available in this release")

	case "host.chatComplete":
		return nil, fmt.Errorf("host.chatComplete is not available in this release")

	default:
		return nil, fmt.Errorf("unknown host method: %s", method)
	}
}
