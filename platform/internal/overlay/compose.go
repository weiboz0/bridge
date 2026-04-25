// Package overlay implements the overlay composition algorithm for teaching units.
// It merges a parent unit's block sequence with a child unit's blocks and
// per-block overrides, producing the final rendered document block list.
//
// This is a pure function with no database access. Callers (store layer)
// are responsible for loading the parent revision blocks, child document
// blocks, and overlay row before calling ComposeDocument.
package overlay

import "encoding/json"

// BlockOverride represents a single override entry in block_overrides JSONB.
// Action must be "hide" or "replace".  Block is required when Action == "replace".
type BlockOverride struct {
	Action string          `json:"action"` // "hide" or "replace"
	Block  json.RawMessage `json:"block,omitempty"`
}

// blockAttrs is a minimal struct for extracting attrs.id and attrs.parentId
// from a raw block JSON node without fully unmarshaling the block.
type blockAttrs struct {
	Type  string `json:"type"`
	Attrs struct {
		ID       string  `json:"id"`
		ParentID *string `json:"parentId"`
	} `json:"attrs"`
}

// extractAttrs parses the minimal attrs fields from a raw block JSON node.
// Returns a zero-value blockAttrs (empty strings, nil parentId) on any parse
// error so the caller can still pass the block through unchanged (defensive).
func extractAttrs(raw json.RawMessage) blockAttrs {
	var b blockAttrs
	_ = json.Unmarshal(raw, &b) // ignore error — caller handles zero-value
	return b
}

// ComposeDocument merges parent blocks with child blocks and overrides.
//
// Algorithm (per spec 012 §Overlay semantics):
//  1. Walk parentBlocks in order.
//  2. For each parent block P:
//     - If overrides[P.attrs.id].action == "hide": skip P.
//     - If overrides[P.attrs.id].action == "replace": emit override.block instead of P.
//     - Otherwise: emit P unchanged.
//     After emitting (or skipping) P, emit all child blocks whose attrs.parentId == P.attrs.id.
//  3. After all parent blocks, emit child blocks where attrs.parentId == null or missing.
//  4. Orphaned child blocks (parentId refers to a parent id that does not appear in
//     parentBlocks) fall through to the end, emitted after null-anchored blocks.
//
// Edge cases handled:
//   - Parent block with no attrs.id: passed through unchanged; child blocks cannot
//     anchor to it (they would need a non-empty parentId matching a real id).
//   - Empty parentBlocks + empty childBlocks: returns empty slice (not nil).
//   - nil overrides map: treated as empty (no overrides).
//
// The returned slice is always non-nil.
func ComposeDocument(
	parentBlocks []json.RawMessage,
	childBlocks []json.RawMessage,
	overrides map[string]BlockOverride,
) []json.RawMessage {
	result := make([]json.RawMessage, 0, len(parentBlocks)+len(childBlocks))

	if overrides == nil {
		overrides = map[string]BlockOverride{}
	}

	// Build a set of all parent block IDs so we can detect orphaned child blocks.
	parentIDSet := make(map[string]struct{}, len(parentBlocks))
	for _, raw := range parentBlocks {
		a := extractAttrs(raw)
		if a.Attrs.ID != "" {
			parentIDSet[a.Attrs.ID] = struct{}{}
		}
	}

	// Index child blocks by their parentId for fast lookup.
	// childByParentID maps parentId → ordered list of child blocks anchored there.
	// nullAnchored collects child blocks with parentId == null / missing.
	// orphaned collects child blocks whose parentId refers to a non-existent parent id.
	childByParentID := make(map[string][]json.RawMessage)
	var nullAnchored []json.RawMessage
	var orphaned []json.RawMessage

	for _, raw := range childBlocks {
		a := extractAttrs(raw)
		if a.Attrs.ParentID == nil || *a.Attrs.ParentID == "" {
			// parentId absent or explicitly null → append to end
			nullAnchored = append(nullAnchored, raw)
		} else {
			pid := *a.Attrs.ParentID
			if _, exists := parentIDSet[pid]; exists {
				childByParentID[pid] = append(childByParentID[pid], raw)
			} else {
				// parentId references a block that is not in the parent document
				orphaned = append(orphaned, raw)
			}
		}
	}

	// Walk parent blocks, applying overrides and emitting anchored children.
	for _, raw := range parentBlocks {
		a := extractAttrs(raw)
		id := a.Attrs.ID

		override, hasOverride := overrides[id]
		if hasOverride {
			switch override.Action {
			case "hide":
				// Skip parent block entirely; still emit any anchored children below.
			case "replace":
				if len(override.Block) > 0 {
					result = append(result, override.Block)
				}
				// If Block is empty/nil on a replace override, treat as hide.
			default:
				// Unknown action — emit parent block unchanged (defensive).
				result = append(result, raw)
			}
		} else {
			// No override — emit parent block unchanged.
			// Parent blocks without an attrs.id are always emitted (they cannot be
			// overridden since there is no id to key on).
			result = append(result, raw)
		}

		// Emit child blocks anchored after this parent block.
		if id != "" {
			result = append(result, childByParentID[id]...)
		}
	}

	// Append null-anchored child blocks (no specific parent anchor).
	result = append(result, nullAnchored...)

	// Append orphaned child blocks (parentId referred to a deleted parent block).
	result = append(result, orphaned...)

	return result
}
