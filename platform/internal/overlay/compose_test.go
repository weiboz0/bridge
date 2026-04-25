package overlay

import (
	"encoding/json"
	"testing"
)

// rawBlock is a helper that returns a json.RawMessage for a block with the
// given attrs. It marshals to compact JSON.
func rawBlock(t *testing.T, blockType string, id string, parentID *string, extra map[string]interface{}) json.RawMessage {
	t.Helper()
	attrs := map[string]interface{}{
		"id": id,
	}
	if parentID != nil {
		attrs["parentId"] = *parentID
	}
	for k, v := range extra {
		attrs[k] = v
	}
	block := map[string]interface{}{
		"type":  blockType,
		"attrs": attrs,
	}
	b, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("rawBlock marshal: %v", err)
	}
	return b
}

// str returns a pointer to a string (for parentID).
func str(s string) *string { return &s }

// blockID extracts attrs.id from a raw block for assertion convenience.
func blockID(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	a := extractAttrs(raw)
	return a.Attrs.ID
}

// blockIDs extracts the ordered list of attrs.id values from a slice of raw blocks.
func blockIDs(t *testing.T, blocks []json.RawMessage) []string {
	t.Helper()
	ids := make([]string, len(blocks))
	for i, b := range blocks {
		ids[i] = blockID(t, b)
	}
	return ids
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// Test 1: No overrides, no child blocks — output equals parent blocks unchanged.
func TestNoOverridesNoChildren(t *testing.T) {
	p1 := rawBlock(t, "prose", "b01", nil, nil)
	p2 := rawBlock(t, "prose", "b02", nil, nil)
	p3 := rawBlock(t, "problem-ref", "b03", nil, nil)

	out := ComposeDocument(
		[]json.RawMessage{p1, p2, p3},
		nil,
		nil,
	)

	if got := blockIDs(t, out); len(got) != 3 {
		t.Fatalf("expected 3 blocks, got %d: %v", len(got), got)
	}
	ids := blockIDs(t, out)
	want := []string{"b01", "b02", "b03"}
	for i, w := range want {
		if ids[i] != w {
			t.Errorf("position %d: want %q, got %q", i, w, ids[i])
		}
	}
}

// Test 2: Hide one block — it is omitted from output.
func TestHideOneBlock(t *testing.T) {
	p1 := rawBlock(t, "prose", "b01", nil, nil)
	p2 := rawBlock(t, "prose", "b02", nil, nil)
	p3 := rawBlock(t, "prose", "b03", nil, nil)

	overrides := map[string]BlockOverride{
		"b02": {Action: "hide"},
	}

	out := ComposeDocument(
		[]json.RawMessage{p1, p2, p3},
		nil,
		overrides,
	)

	ids := blockIDs(t, out)
	if len(ids) != 2 {
		t.Fatalf("expected 2 blocks after hide, got %d: %v", len(ids), ids)
	}
	if ids[0] != "b01" || ids[1] != "b03" {
		t.Errorf("expected [b01 b03], got %v", ids)
	}
}

// Test 3: Replace one block — the replacement block appears in the output.
func TestReplaceOneBlock(t *testing.T) {
	p1 := rawBlock(t, "prose", "b01", nil, nil)
	p2 := rawBlock(t, "prose", "b02", nil, nil)

	replacement := rawBlock(t, "prose", "b02-replacement", nil, nil)
	overrides := map[string]BlockOverride{
		"b02": {Action: "replace", Block: replacement},
	}

	out := ComposeDocument(
		[]json.RawMessage{p1, p2},
		nil,
		overrides,
	)

	ids := blockIDs(t, out)
	if len(ids) != 2 {
		t.Fatalf("expected 2 blocks, got %d: %v", len(ids), ids)
	}
	if ids[0] != "b01" {
		t.Errorf("position 0: want b01, got %q", ids[0])
	}
	if ids[1] != "b02-replacement" {
		t.Errorf("position 1: want b02-replacement (replacement), got %q", ids[1])
	}
}

// Test 4: Child block anchored after a specific parent block — emitted right after it.
func TestChildAnchoredAfterParentBlock(t *testing.T) {
	p1 := rawBlock(t, "prose", "b01", nil, nil)
	p2 := rawBlock(t, "prose", "b02", nil, nil)
	p3 := rawBlock(t, "prose", "b03", nil, nil)

	// c1 anchors after b02
	c1 := rawBlock(t, "teacher-note", "c1", str("b02"), nil)

	out := ComposeDocument(
		[]json.RawMessage{p1, p2, p3},
		[]json.RawMessage{c1},
		nil,
	)

	ids := blockIDs(t, out)
	// Expected: b01, b02, c1, b03
	want := []string{"b01", "b02", "c1", "b03"}
	if len(ids) != len(want) {
		t.Fatalf("expected %v, got %v", want, ids)
	}
	for i, w := range want {
		if ids[i] != w {
			t.Errorf("position %d: want %q, got %q", i, w, ids[i])
		}
	}
}

// Test 5: Child block with parentId == null → appended at the document end.
func TestChildWithNullParentIDAppendedAtEnd(t *testing.T) {
	p1 := rawBlock(t, "prose", "b01", nil, nil)
	p2 := rawBlock(t, "prose", "b02", nil, nil)

	// Explicitly pass a block without parentId in attrs (parentId absent = null)
	nullBlock := rawBlock(t, "prose", "c-null", nil, nil)

	out := ComposeDocument(
		[]json.RawMessage{p1, p2},
		[]json.RawMessage{nullBlock},
		nil,
	)

	ids := blockIDs(t, out)
	want := []string{"b01", "b02", "c-null"}
	if len(ids) != len(want) {
		t.Fatalf("expected %v, got %v", want, ids)
	}
	for i, w := range want {
		if ids[i] != w {
			t.Errorf("position %d: want %q, got %q", i, w, ids[i])
		}
	}
}

// Test 6: Orphaned child block (parentId refers to a non-existent parent) → falls to end.
func TestOrphanedChildFallsToEnd(t *testing.T) {
	p1 := rawBlock(t, "prose", "b01", nil, nil)
	p2 := rawBlock(t, "prose", "b02", nil, nil)

	// c-orphan has parentId="deleted-block" which is NOT in parentBlocks
	orphan := rawBlock(t, "prose", "c-orphan", str("deleted-block"), nil)

	out := ComposeDocument(
		[]json.RawMessage{p1, p2},
		[]json.RawMessage{orphan},
		nil,
	)

	ids := blockIDs(t, out)
	// Orphan should fall to the end, after null-anchored blocks
	want := []string{"b01", "b02", "c-orphan"}
	if len(ids) != len(want) {
		t.Fatalf("expected %v, got %v", want, ids)
	}
	for i, w := range want {
		if ids[i] != w {
			t.Errorf("position %d: want %q, got %q", i, w, ids[i])
		}
	}
}

// Test 7: Empty parent + empty child → empty output (not nil, but zero-length).
func TestEmptyParentAndChild(t *testing.T) {
	out := ComposeDocument(nil, nil, nil)
	if out == nil {
		t.Fatal("expected non-nil slice for empty inputs")
	}
	if len(out) != 0 {
		t.Fatalf("expected empty slice, got %d elements", len(out))
	}
}

// Test 8: Parent block without attrs.id — passed through unchanged.
func TestParentBlockWithoutID(t *testing.T) {
	// A block that omits attrs entirely (minimal JSON)
	noIDBlock := json.RawMessage(`{"type":"prose","attrs":{}}`)
	p2 := rawBlock(t, "prose", "b02", nil, nil)

	out := ComposeDocument(
		[]json.RawMessage{noIDBlock, p2},
		nil,
		nil,
	)

	if len(out) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(out))
	}
	// First block has empty id (passed through unchanged)
	if id := blockID(t, out[0]); id != "" {
		t.Errorf("expected empty id for no-id block, got %q", id)
	}
	if id := blockID(t, out[1]); id != "b02" {
		t.Errorf("expected b02, got %q", id)
	}
}

// Test 9: Mixed scenario — hide + replace + anchored children + null-anchored + orphaned.
func TestMixedScenario(t *testing.T) {
	// Parent blocks: p1, p2 (will be hidden), p3 (will be replaced), p4
	p1 := rawBlock(t, "prose", "b01", nil, nil)
	p2 := rawBlock(t, "prose", "b02", nil, nil) // hidden
	p3 := rawBlock(t, "prose", "b03", nil, nil) // replaced
	p4 := rawBlock(t, "prose", "b04", nil, nil)

	// Child blocks:
	// c-after-b01: anchored after b01
	cAfterB01 := rawBlock(t, "teacher-note", "c-after-b01", str("b01"), nil)
	// c-after-b04: anchored after b04
	cAfterB04 := rawBlock(t, "live-cue", "c-after-b04", str("b04"), nil)
	// c-null: no anchor (append to end)
	cNull := rawBlock(t, "prose", "c-null", nil, nil)
	// c-orphan: references a deleted parent block
	cOrphan := rawBlock(t, "prose", "c-orphan", str("deleted-b99"), nil)

	replacement := rawBlock(t, "prose", "b03-new", nil, nil)
	overrides := map[string]BlockOverride{
		"b02": {Action: "hide"},
		"b03": {Action: "replace", Block: replacement},
	}

	out := ComposeDocument(
		[]json.RawMessage{p1, p2, p3, p4},
		[]json.RawMessage{cAfterB01, cAfterB04, cNull, cOrphan},
		overrides,
	)

	// Expected order:
	// b01, c-after-b01  (b01 emitted + anchored child)
	// (b02 hidden — nothing emitted)
	// b03-new           (b03 replaced)
	// b04, c-after-b04  (b04 emitted + anchored child)
	// c-null            (null-anchored, appended at end)
	// c-orphan          (orphan, appended last)
	want := []string{"b01", "c-after-b01", "b03-new", "b04", "c-after-b04", "c-null", "c-orphan"}

	ids := blockIDs(t, out)
	if len(ids) != len(want) {
		t.Fatalf("expected %v (%d), got %v (%d)", want, len(want), ids, len(ids))
	}
	for i, w := range want {
		if ids[i] != w {
			t.Errorf("position %d: want %q, got %q", i, w, ids[i])
		}
	}
}

// Test 10: Multiple child blocks anchored after the same parent block — order preserved.
func TestMultipleChildrenAnchoredAfterSameParent(t *testing.T) {
	p1 := rawBlock(t, "prose", "b01", nil, nil)
	p2 := rawBlock(t, "prose", "b02", nil, nil)

	c1 := rawBlock(t, "teacher-note", "c1", str("b01"), nil)
	c2 := rawBlock(t, "teacher-note", "c2", str("b01"), nil)
	c3 := rawBlock(t, "teacher-note", "c3", str("b01"), nil)

	out := ComposeDocument(
		[]json.RawMessage{p1, p2},
		[]json.RawMessage{c1, c2, c3},
		nil,
	)

	want := []string{"b01", "c1", "c2", "c3", "b02"}
	ids := blockIDs(t, out)
	if len(ids) != len(want) {
		t.Fatalf("expected %v, got %v", want, ids)
	}
	for i, w := range want {
		if ids[i] != w {
			t.Errorf("position %d: want %q, got %q", i, w, ids[i])
		}
	}
}

// Test 11: Nil overrides map is treated as empty (no panics, all parent blocks emitted).
func TestNilOverridesTreatedAsEmpty(t *testing.T) {
	p1 := rawBlock(t, "prose", "b01", nil, nil)

	out := ComposeDocument([]json.RawMessage{p1}, nil, nil)
	ids := blockIDs(t, out)
	if len(ids) != 1 || ids[0] != "b01" {
		t.Errorf("expected [b01], got %v", ids)
	}
}

// Test 12: Replace override with empty/nil Block is treated as hide (defensive).
func TestReplaceWithEmptyBlockActsAsHide(t *testing.T) {
	p1 := rawBlock(t, "prose", "b01", nil, nil)
	p2 := rawBlock(t, "prose", "b02", nil, nil)

	// Replace override with no Block payload — should omit b02
	overrides := map[string]BlockOverride{
		"b02": {Action: "replace", Block: nil},
	}

	out := ComposeDocument(
		[]json.RawMessage{p1, p2},
		nil,
		overrides,
	)

	ids := blockIDs(t, out)
	// b02 should be omitted because replace.Block is nil/empty
	if len(ids) != 1 || ids[0] != "b01" {
		t.Errorf("expected [b01], got %v", ids)
	}
}

// Test 13: Null-anchored and orphaned blocks both fall to end, null before orphaned.
func TestNullAnchoredBeforeOrphaned(t *testing.T) {
	p1 := rawBlock(t, "prose", "b01", nil, nil)

	cNull := rawBlock(t, "prose", "c-null", nil, nil)
	cOrphan := rawBlock(t, "prose", "c-orphan", str("non-existent"), nil)

	out := ComposeDocument(
		[]json.RawMessage{p1},
		[]json.RawMessage{cNull, cOrphan},
		nil,
	)

	want := []string{"b01", "c-null", "c-orphan"}
	ids := blockIDs(t, out)
	if len(ids) != len(want) {
		t.Fatalf("expected %v, got %v", want, ids)
	}
	for i, w := range want {
		if ids[i] != w {
			t.Errorf("position %d: want %q, got %q", i, w, ids[i])
		}
	}
}
