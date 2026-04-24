# 032 — Teaching Unit: Migration of Existing Topics

**Goal:** Convert each existing `topic.lessonContent` + its `topic_problems` rows into a teaching unit (`scope='org'`, `status='classroom_ready'`). Sessions and assignments continue to resolve through a compatibility shim that also returns a `unit_id`. Lands early so plans 033 onward operate on a library with real content.

**Spec:** `docs/specs/012-teaching-units.md` — §How existing materials flow in

**Depends On:** Plan 031

**Status:** Not started

---

## Scope

- One-time migration script converting `topics.lessonContent` into `teaching_units` + `unit_documents`
- Each topic's `topic_problems` rows become `problem-ref` blocks in the unit document
- Compatibility shim: `session_topics` and assignment lookups can resolve a `unit_id` alongside the existing `topic_id`
- Existing flows keep working — no breaking changes to session or assignment surfaces

## Tasks

To be written when this plan is picked up for implementation.
