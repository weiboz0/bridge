"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import dynamic from "next/dynamic";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { ApiError } from "@/lib/api-client";
import type { ProblemDetailData } from "./teacher-problem-detail";

// Plan 066 phase 3 — shared create/edit form for the teacher problem
// bank. Decisions §2: useState + manual onSubmit (no react-hook-form).
//
// scopeId auto-fill rules (Decisions §8):
//   - personal → identity.userId
//   - org      → user picks from teacher/org_admin memberships
//   - platform → null (backend authorizes by IsPlatformAdmin only;
//                see platform/internal/handlers/problem_access.go:28)
//
// Slug (Decisions §5) is optional with title-derived auto-suggest;
// user-typed values override the suggestion.
//
// Tags (Decisions §9) are a free-text chip input with a 64-char
// per-tag client-side limit. No normalization beyond trim.

// Monaco loads client-only — `next/dynamic` with ssr:false avoids the
// "window is not defined" boot error and shrinks the SSR HTML.
const CodeEditor = dynamic(
  () => import("@/components/editor/code-editor").then((m) => m.CodeEditor),
  { ssr: false, loading: () => <EditorSkeleton /> },
);

interface OrgMembership {
  orgId: string;
  orgName: string;
  role: string;
  status: string;
  orgStatus: string;
}

interface IdentityShape {
  userId: string;
  isPlatformAdmin: boolean;
}

type FormScope = "personal" | "org" | "platform";

const STARTER_LANGUAGES: { value: string; label: string; monaco: string }[] = [
  { value: "python", label: "Python", monaco: "python" },
  { value: "javascript", label: "JavaScript", monaco: "javascript" },
  { value: "typescript", label: "TypeScript", monaco: "typescript" },
];

interface Props {
  mode: "create" | "edit";
  identity: IdentityShape;
  initial?: ProblemDetailData;
}

export function ProblemForm({ mode, identity, initial }: Props) {
  const router = useRouter();

  const [orgs, setOrgs] = useState<OrgMembership[]>([]);
  const [orgsLoading, setOrgsLoading] = useState(true);

  const [title, setTitle] = useState(initial?.title ?? "");
  const [slug, setSlug] = useState(initial?.slug ?? "");
  const [slugTouched, setSlugTouched] = useState(Boolean(initial?.slug));
  const [description, setDescription] = useState(initial?.description ?? "");
  const [descriptionPreview, setDescriptionPreview] = useState(false);
  const [difficulty, setDifficulty] = useState(initial?.difficulty || "easy");
  const [gradeLevel, setGradeLevel] = useState(initial?.gradeLevel ?? "");
  const [scope, setScope] = useState<FormScope>(
    (initial?.scope as FormScope) ?? "personal",
  );
  const [orgId, setOrgId] = useState(
    initial?.scope === "org" && initial.scopeId ? initial.scopeId : "",
  );
  const [tags, setTags] = useState<string[]>(initial?.tags ?? []);
  const [tagDraft, setTagDraft] = useState("");
  const [tagError, setTagError] = useState<string | null>(null);

  const initialStarter = initial?.starterCode ?? {};
  const [starterCode, setStarterCode] = useState<Record<string, string>>(() => {
    // Seed all known languages with the existing values (or empty
    // string) so each tab is always editable. Languages present in
    // `initial` that we don't know about are preserved verbatim so an
    // edit doesn't silently drop them.
    const seeded: Record<string, string> = {};
    for (const lang of STARTER_LANGUAGES) {
      seeded[lang.value] = initialStarter[lang.value] ?? "";
    }
    for (const [k, v] of Object.entries(initialStarter)) {
      if (!(k in seeded)) seeded[k] = v;
    }
    return seeded;
  });
  const [activeLang, setActiveLang] = useState<string>(
    STARTER_LANGUAGES[0].value,
  );

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  // Slug auto-suggest: derive from title until the user manually edits
  // the slug field. After touch, the suggestion stops overwriting.
  useEffect(() => {
    if (slugTouched) return;
    setSlug(slugify(title));
  }, [title, slugTouched]);

  useEffect(() => {
    let cancelled = false;
    async function loadOrgs() {
      try {
        const res = await fetch("/api/orgs");
        if (cancelled) return;
        if (!res.ok) {
          setOrgsLoading(false);
          return;
        }
        const data: OrgMembership[] = await res.json();
        const teacherOrgs = data.filter(
          (m) =>
            m.status === "active" &&
            m.orgStatus === "active" &&
            (m.role === "teacher" || m.role === "org_admin"),
        );
        const byOrg = new Map<string, OrgMembership>();
        for (const m of teacherOrgs) {
          if (!byOrg.has(m.orgId)) byOrg.set(m.orgId, m);
        }
        const unique = Array.from(byOrg.values());
        if (cancelled) return;
        setOrgs(unique);
        // Preselect on create only — edit mode already has orgId from
        // `initial`.
        if (mode === "create" && unique.length > 0 && !orgId) {
          setOrgId(unique[0].orgId);
        }
      } catch {
        // Network failure is non-fatal — user can still pick personal.
      } finally {
        if (!cancelled) setOrgsLoading(false);
      }
    }
    loadOrgs();
    return () => {
      cancelled = true;
    };
    // mode + initial orgId only: orgs load once per mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const scopeOptions = useMemo(() => {
    const opts: { value: FormScope; label: string }[] = [
      { value: "personal", label: "Personal" },
    ];
    if (orgs.length > 0) opts.push({ value: "org", label: "Organization" });
    if (identity.isPlatformAdmin) opts.push({ value: "platform", label: "Platform" });
    return opts;
  }, [orgs.length, identity.isPlatformAdmin]);

  function addTag() {
    const trimmed = tagDraft.trim();
    if (!trimmed) return;
    if (trimmed.length > 64) {
      setTagError("Tag must be 64 characters or fewer");
      return;
    }
    if (tags.includes(trimmed)) {
      setTagError("Tag already added");
      return;
    }
    setTags([...tags, trimmed]);
    setTagDraft("");
    setTagError(null);
  }

  function removeTag(t: string) {
    setTags(tags.filter((x) => x !== t));
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setFieldErrors({});
    setError(null);

    const errors: Record<string, string> = {};
    if (!title.trim()) errors.title = "Title is required";
    if (title.trim().length > 255) errors.title = "Title must be 255 characters or fewer";
    if (scope === "org" && !orgId) errors.scope = "Pick an organization";
    if (scope === "personal" && !identity.userId) errors.scope = "Sign in required";
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors);
      return;
    }

    // Drop empty starter-code entries — backend stores the map as-is,
    // so empty strings would persist as "this language is supported
    // but empty", which is not what an empty tab means.
    const starterPayload: Record<string, string> = {};
    for (const [k, v] of Object.entries(starterCode)) {
      if (v.trim() !== "") starterPayload[k] = v;
    }

    const scopeIdForRequest =
      scope === "personal"
        ? identity.userId
        : scope === "org"
          ? orgId
          : null;

    const body = {
      scope,
      scopeId: scopeIdForRequest,
      title: title.trim(),
      slug: slug.trim() || null,
      description,
      starterCode: starterPayload,
      difficulty,
      gradeLevel: gradeLevel || null,
      tags,
    };

    setSubmitting(true);
    try {
      if (mode === "create") {
        const res = await fetch("/api/problems", {
          method: "POST",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        if (!res.ok) throw await readError(res);
        const created = (await res.json()) as { id: string };
        router.push(`/teacher/problems/${created.id}`);
      } else {
        if (!initial) throw new Error("Edit mode requires initial data");
        // PATCH only sends the fields that may have changed. Scope and
        // scopeId stay fixed in edit mode — moving a problem between
        // scopes belongs to a future "transfer" affordance, not this
        // form. The store struct uses pointer fields so unset = unchanged.
        const editBody = {
          title: body.title,
          slug: body.slug,
          description: body.description,
          starterCode: body.starterCode,
          difficulty: body.difficulty,
          gradeLevel: body.gradeLevel,
          tags: body.tags,
        };
        const res = await fetch(`/api/problems/${initial.id}`, {
          method: "PATCH",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(editBody),
        });
        if (!res.ok) throw await readError(res);
        router.push(`/teacher/problems/${initial.id}`);
      }
      router.refresh();
    } catch (e) {
      if (e instanceof ApiError) {
        // Plan 071 — backend now returns 409 + {"field":"slug"} on slug
        // unique-violations. Pin those inline instead of the banner so
        // the user can see exactly which field needs fixing.
        const apiBody = e.body as { field?: string } | null;
        if (e.status === 409 && apiBody?.field === "slug") {
          setFieldErrors((prev) => ({ ...prev, slug: e.message }));
          setError(null);
        } else {
          setError(e.message);
        }
      } else if (e instanceof Error) {
        setError(e.message);
      } else {
        setError(String(e));
      }
      setSubmitting(false);
    }
  }

  const scopeFixed = mode === "edit"; // §Decisions §8 — scope is immutable after create

  return (
    <div className="p-6 max-w-3xl">
      <h1 className="text-2xl font-bold mb-1">
        {mode === "create" ? "Create Problem" : "Edit Problem"}
      </h1>
      <p className="text-sm text-muted-foreground mb-6">
        {mode === "create"
          ? "Drafts are private until you publish. You can edit everything later."
          : "Changes apply immediately. Status (draft / published / archived) is changed from the detail page."}
      </p>

      <form onSubmit={handleSubmit} className="space-y-6">
        {/* Title */}
        <div className="space-y-1.5">
          <Label htmlFor="title">Title</Label>
          <Input
            id="title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="e.g. Two Sum"
            required
            maxLength={255}
            aria-invalid={Boolean(fieldErrors.title)}
          />
          {fieldErrors.title && (
            <p className="text-sm text-destructive">{fieldErrors.title}</p>
          )}
        </div>

        {/* Slug */}
        <div className="space-y-1.5">
          <Label htmlFor="slug">Slug</Label>
          <Input
            id="slug"
            value={slug}
            onChange={(e) => {
              setSlugTouched(true);
              setSlug(e.target.value);
              if (fieldErrors.slug) {
                setFieldErrors((prev) => {
                  const next = { ...prev };
                  delete next.slug;
                  return next;
                });
              }
            }}
            placeholder="auto-generated from title"
            aria-invalid={Boolean(fieldErrors.slug)}
          />
          {fieldErrors.slug && (
            <p className="text-sm text-destructive">{fieldErrors.slug}</p>
          )}
          <p className="text-xs text-muted-foreground">
            Optional. Used in URLs and search. Lowercase letters, digits, and
            hyphens. Leave blank to auto-generate from the title.
          </p>
        </div>

        {/* Scope */}
        <div className="space-y-1.5">
          <Label htmlFor="scope">Scope</Label>
          <select
            id="scope"
            value={scope}
            onChange={(e) => setScope(e.target.value as FormScope)}
            disabled={scopeFixed}
            className="flex h-9 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-60"
            aria-invalid={Boolean(fieldErrors.scope)}
          >
            {scopeOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
            {/* In edit mode, the current scope must remain selectable
                even if it isn't in the create-time options (e.g., a
                platform-scope problem viewed by a former platform
                admin). */}
            {scopeFixed &&
              !scopeOptions.find((o) => o.value === scope) && (
                <option value={scope}>{scope}</option>
              )}
          </select>
          {scopeFixed && (
            <p className="text-xs text-muted-foreground">
              Scope cannot be changed after creation.
            </p>
          )}
          {fieldErrors.scope && (
            <p className="text-sm text-destructive">{fieldErrors.scope}</p>
          )}
        </div>

        {/* Org picker — only when scope=org */}
        {scope === "org" && (
          <div className="space-y-1.5">
            <Label htmlFor="orgId">Organization</Label>
            {orgsLoading ? (
              <p className="text-sm text-muted-foreground">Loading organizations…</p>
            ) : orgs.length === 0 ? (
              <p className="text-sm text-destructive">
                No organizations found. You must be a teacher or org admin in
                an active organization to create org-scoped problems.
              </p>
            ) : (
              <select
                id="orgId"
                value={orgId}
                onChange={(e) => setOrgId(e.target.value)}
                disabled={scopeFixed}
                className="flex h-9 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-60"
              >
                {orgs.map((org) => (
                  <option key={org.orgId} value={org.orgId}>
                    {org.orgName}
                  </option>
                ))}
                {scopeFixed && !orgs.find((o) => o.orgId === orgId) && orgId && (
                  <option value={orgId}>{orgId}</option>
                )}
              </select>
            )}
          </div>
        )}

        {/* Difficulty + grade level (side-by-side on wider screens) */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-1.5">
            <Label htmlFor="difficulty">Difficulty</Label>
            <select
              id="difficulty"
              value={difficulty}
              onChange={(e) => setDifficulty(e.target.value)}
              className="flex h-9 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <option value="easy">Easy</option>
              <option value="medium">Medium</option>
              <option value="hard">Hard</option>
            </select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="gradeLevel">Grade level</Label>
            <select
              id="gradeLevel"
              value={gradeLevel}
              onChange={(e) => setGradeLevel(e.target.value)}
              className="flex h-9 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <option value="">Not specified</option>
              <option value="K-5">K-5</option>
              <option value="6-8">6-8</option>
              <option value="9-12">9-12</option>
            </select>
          </div>
        </div>

        {/* Description (markdown) */}
        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <Label htmlFor="description">Description (markdown)</Label>
            <button
              type="button"
              onClick={() => setDescriptionPreview((v) => !v)}
              className="text-xs text-muted-foreground hover:text-foreground"
              aria-pressed={descriptionPreview}
            >
              {descriptionPreview ? "Edit" : "Preview"}
            </button>
          </div>
          {descriptionPreview ? (
            <div className="min-h-[16rem] w-full rounded-lg border border-input bg-zinc-50/50 px-3 py-2">
              {description.trim() ? (
                <div className="prose prose-sm max-w-none">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>
                    {description}
                  </ReactMarkdown>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground italic">
                  Nothing to preview yet — write some markdown above.
                </p>
              )}
            </div>
          ) : (
            <textarea
              id="description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={10}
              placeholder={"## Problem\n\nDescribe the problem, input/output format, constraints, and examples."}
              className="flex w-full rounded-lg border border-input bg-transparent px-3 py-2 font-mono text-sm leading-relaxed outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 resize-y"
            />
          )}
          <p className="text-xs text-muted-foreground">
            Toggle Preview to see the rendered markdown. The detail page uses
            the same renderer.
          </p>
        </div>

        {/* Starter code */}
        <div className="space-y-2">
          <Label>Starter code</Label>
          <div className="flex items-center gap-1 border-b border-zinc-200">
            {STARTER_LANGUAGES.map((lang) => (
              <button
                key={lang.value}
                type="button"
                onClick={() => setActiveLang(lang.value)}
                className={`px-3 py-1.5 text-xs font-medium border-b-2 -mb-px transition-colors ${
                  activeLang === lang.value
                    ? "border-primary text-primary"
                    : "border-transparent text-muted-foreground hover:text-foreground"
                }`}
              >
                {lang.label}
              </button>
            ))}
          </div>
          <div className="h-72 overflow-hidden rounded-lg border border-zinc-200">
            <CodeEditor
              key={activeLang}
              initialCode={starterCode[activeLang] ?? ""}
              language={
                STARTER_LANGUAGES.find((l) => l.value === activeLang)?.monaco ??
                "plaintext"
              }
              onChange={(code) =>
                setStarterCode((prev) => ({ ...prev, [activeLang]: code }))
              }
            />
          </div>
          <p className="text-xs text-muted-foreground">
            Empty tabs are not saved. Add code only for the languages you support.
          </p>
        </div>

        {/* Tags */}
        <div className="space-y-1.5">
          <Label htmlFor="tagDraft">Tags</Label>
          <div className="flex flex-wrap items-center gap-2">
            {tags.map((t) => (
              <Badge
                key={t}
                variant="secondary"
                className="gap-1 pr-1.5 font-normal"
              >
                {t}
                <button
                  type="button"
                  onClick={() => removeTag(t)}
                  className="ml-1 rounded-sm text-muted-foreground hover:text-foreground"
                  aria-label={`Remove tag ${t}`}
                >
                  ×
                </button>
              </Badge>
            ))}
          </div>
          <div className="flex gap-2">
            <Input
              id="tagDraft"
              value={tagDraft}
              onChange={(e) => {
                setTagDraft(e.target.value);
                if (tagError) setTagError(null);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === ",") {
                  e.preventDefault();
                  addTag();
                }
              }}
              maxLength={64}
              placeholder="Type a tag and press Enter"
            />
            <Button type="button" variant="outline" onClick={addTag}>
              Add
            </Button>
          </div>
          {tagError && <p className="text-sm text-destructive">{tagError}</p>}
          <p className="text-xs text-muted-foreground">
            Up to 64 characters per tag. Used for filtering and search.
          </p>
        </div>

        {/* Submission errors */}
        {error && (
          <div
            role="alert"
            className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
          >
            {error}
          </div>
        )}

        <div className="flex gap-3 pt-2">
          <Button
            type="submit"
            disabled={submitting || (scope === "org" && orgsLoading)}
          >
            {submitting
              ? mode === "create"
                ? "Creating…"
                : "Saving…"
              : mode === "create"
                ? "Create problem"
                : "Save changes"}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => router.back()}
            disabled={submitting}
          >
            Cancel
          </Button>
        </div>
      </form>
    </div>
  );
}

function EditorSkeleton() {
  return (
    <div className="h-full w-full animate-pulse bg-zinc-100" />
  );
}

function slugify(title: string): string {
  return title
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
}

async function readError(res: Response): Promise<ApiError> {
  // Plan 071 widened the body shape — the handler may attach a `field`
  // hint on validation/conflict errors so the form can pin the message
  // inline (currently only slug-conflict 409 uses this).
  const body = (await res.json().catch(() => null)) as
    | { error?: string; field?: string }
    | null;
  // 403 keeps a friendlier message than the raw "not authorized for scope"
  // the Go handler returns; everything else passes through.
  if (res.status === 403) {
    return new ApiError(
      403,
      "You don't have permission to save this problem under the selected scope.",
      body,
    );
  }
  return new ApiError(
    res.status,
    body?.error ?? `Request failed: ${res.status}`,
    body,
  );
}
