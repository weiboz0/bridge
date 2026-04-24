# 036 — Teaching Unit: Discovery + Libraries

**Goal:** Build the teacher-facing library surfaces (platform / org / personal), composite search (pgvector semantic + Postgres FTS + structured filters), quality-signal capture, and unit collections.

**Spec:** `docs/specs/012-teaching-units.md` — §Discovery layers, §Quality signals, §unit_collections

**Depends On:** Plan 032 (real migrated content to search), Plan 033 (lifecycle statuses to filter)

**Status:** Not started

---

## Scope

- pgvector on unit title + summary + first N prose blocks for semantic similarity
- Postgres full-text search on title + summary for keyword matching
- Structured filters: grade level, subject tags, standards tags, estimated minutes, scope, status
- Composite ranking: blend semantic score + FTS score + quality signals
- Library surfaces: "My Drafts", "Org Library", "Platform Library", "Community" (stub)
- `unit_collections` + `unit_collection_items` tables for curated sequences
- Quality signal capture: class usages, student completion rate, student success rate, teacher ratings
- Scoring function deferred to v2 — capture signals first, derive ranking later

## Tasks

To be written when this plan is picked up for implementation.
