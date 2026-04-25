// Package projection implements the per-role render projection pipeline for
// teaching unit documents. The pipeline is a pure function — no DB access —
// that filters a unit document's top-level blocks based on viewer role and
// attempt state, per spec 012's projection table.
package projection

import "encoding/json"

// ViewerRole identifies the role of the user viewing the projected document.
type ViewerRole string

const (
	RoleStudent ViewerRole = "student"
	RoleTeacher ViewerRole = "teacher"
	RoleAdmin   ViewerRole = "platform_admin"
)

// AttemptState represents a student's attempt state for a problem block.
type AttemptState string

const (
	AttemptNotStarted AttemptState = "not_started"
	AttemptSubmitted  AttemptState = "submitted"
	AttemptPassed     AttemptState = "passed"
	AttemptFailed     AttemptState = "failed"
)

// alwaysInclude is the set of block types that are shown to all viewers
// regardless of role.
var alwaysInclude = map[string]bool{
	"prose":          true,
	"code-snippet":   true,
	"media-embed":    true,
	"paragraph":      true,
	"heading":        true,
	"bulletList":     true,
	"orderedList":    true,
	"listItem":       true,
	"codeBlock":      true,
	"blockquote":     true,
	"horizontalRule": true,
	"hardBreak":      true,
	"test-case-ref":  true,
}

// isPrivileged returns true for teacher and platform_admin roles.
func isPrivileged(role ViewerRole) bool {
	return role == RoleTeacher || role == RoleAdmin
}

// blockHeader is the minimal structure we decode from each block to determine
// its type and relevant attributes for projection decisions.
type blockHeader struct {
	Type  string `json:"type"`
	Attrs struct {
		ID         string `json:"id"`
		Visibility string `json:"visibility"` // problem-ref
		Reveal     string `json:"reveal"`     // solution-ref
	} `json:"attrs"`
}

// ProjectBlocks filters a unit document's top-level blocks for the given
// viewer. attemptStates maps block IDs (for solution-ref blocks) to their
// attempt state, enabling solution reveal logic. Blocks not matching the
// viewer's projection rules are omitted entirely.
//
// The function is pure — it performs no I/O and has no side effects.
func ProjectBlocks(
	blocks []json.RawMessage,
	role ViewerRole,
	attemptStates map[string]AttemptState,
) []json.RawMessage {
	if len(blocks) == 0 {
		return []json.RawMessage{}
	}

	result := make([]json.RawMessage, 0, len(blocks))

	for _, raw := range blocks {
		var hdr blockHeader
		// If we can't parse the block header, skip it defensively.
		if err := json.Unmarshal(raw, &hdr); err != nil {
			continue
		}

		if shouldInclude(hdr, role, attemptStates) {
			result = append(result, raw)
		}
	}

	return result
}

// shouldInclude determines whether a single block should be included in the
// projected output based on its type, the viewer role, and attempt states.
func shouldInclude(hdr blockHeader, role ViewerRole, attemptStates map[string]AttemptState) bool {
	// Always-include block types are shown to everyone.
	if alwaysInclude[hdr.Type] {
		return true
	}

	switch hdr.Type {
	case "problem-ref":
		return shouldIncludeProblemRef(hdr, role)
	case "teacher-note":
		return isPrivileged(role)
	case "live-cue":
		return isPrivileged(role)
	case "solution-ref":
		return shouldIncludeSolutionRef(hdr, role, attemptStates)
	case "assignment-variant":
		return isPrivileged(role)
	default:
		// Unknown block types: include for privileged users, omit for students.
		// This is defensive — unknown types shouldn't reach here if the
		// allowlist is enforced on save, but we handle it gracefully.
		return isPrivileged(role)
	}
}

// shouldIncludeProblemRef determines visibility for problem-ref blocks.
// Visible if attrs.visibility="always" or if the viewer is teacher/admin.
// "when-unit-active" requires session binding which doesn't exist yet,
// so it's treated as teacher/admin-only.
func shouldIncludeProblemRef(hdr blockHeader, role ViewerRole) bool {
	if isPrivileged(role) {
		return true
	}
	// Student: only if visibility is "always"
	return hdr.Attrs.Visibility == "always"
}

// shouldIncludeSolutionRef determines visibility for solution-ref blocks.
// For teacher/admin: always visible.
// For students: visible if reveal="always", or reveal="after-submit" AND the
// associated attempt state is submitted, passed, or failed.
func shouldIncludeSolutionRef(hdr blockHeader, role ViewerRole, attemptStates map[string]AttemptState) bool {
	if isPrivileged(role) {
		return true
	}

	switch hdr.Attrs.Reveal {
	case "always":
		return true
	case "after-submit":
		if attemptStates == nil {
			return false
		}
		state, ok := attemptStates[hdr.Attrs.ID]
		if !ok {
			return false
		}
		return state == AttemptSubmitted || state == AttemptPassed || state == AttemptFailed
	default:
		// Unknown reveal value — hide from students defensively.
		return false
	}
}
