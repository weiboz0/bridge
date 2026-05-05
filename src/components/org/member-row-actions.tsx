"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";

interface MemberInfo {
  membershipId: string;
  userId: string;
  name: string;
  email: string;
  status: string; // "active" | "pending" | "suspended"
}

interface Props {
  orgId: string;
  member: MemberInfo;
  isSelf: boolean; // true when member.userId === identity.userId
}

// ─── Update-status modal ─────────────────────────────────────────────────────

interface UpdateStatusModalProps {
  orgId: string;
  member: MemberInfo;
  isSelf: boolean;
  onClose: () => void;
  onDone: () => void;
}

function UpdateStatusModal({
  orgId,
  member,
  isSelf,
  onClose,
  onDone,
}: UpdateStatusModalProps) {
  const [status, setStatus] = useState(member.status);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const backdropRef = useRef<HTMLDivElement | null>(null);
  const router = useRouter();

  // Close on Escape
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const res = await fetch(
        `/api/orgs/${orgId}/members/${member.membershipId}`,
        {
          method: "PATCH",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ status }),
        },
      );
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as
          | { error?: string }
          | null;
        if (res.status === 403) {
          setError("You don't have permission to do that.");
        } else if (res.status === 404) {
          setError("Membership no longer exists; refreshing.");
          router.refresh();
          onDone();
        } else {
          setError(body?.error ?? `Request failed: ${res.status}`);
        }
        return;
      }
      router.refresh();
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      ref={backdropRef}
      role="dialog"
      aria-modal="true"
      aria-label="Update member status"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        if (e.target === backdropRef.current) onClose();
      }}
    >
      <div className="w-full max-w-sm rounded-lg bg-background p-6 shadow-xl">
        <h2 className="text-lg font-semibold mb-1">Update status</h2>
        <p className="text-sm text-muted-foreground mb-4">
          Change the membership status for{" "}
          <span className="font-medium">{member.name}</span>.
        </p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <label
              htmlFor="member-status-select"
              className="block text-sm font-medium"
            >
              Status
            </label>
            <select
              id="member-status-select"
              value={status}
              onChange={(e) => setStatus(e.target.value)}
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            >
              <option value="active">active</option>
              <option value="pending">pending</option>
              <option
                value="suspended"
                disabled={isSelf}
                title={
                  isSelf ? "You can't suspend yourself." : undefined
                }
              >
                suspended{isSelf ? " (unavailable — can't suspend yourself)" : ""}
              </option>
            </select>
          </div>

          {error && (
            <div
              role="alert"
              className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
            >
              {error}
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="rounded-md border border-input bg-background px-4 py-2 text-sm font-medium hover:bg-muted disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {submitting ? "Saving…" : "Save"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

// ─── Remove confirm dialog ────────────────────────────────────────────────────

interface RemoveDialogProps {
  orgId: string;
  member: MemberInfo;
  onClose: () => void;
  onDone: () => void;
}

function RemoveDialog({
  orgId,
  member,
  onClose,
  onDone,
}: RemoveDialogProps) {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const backdropRef = useRef<HTMLDivElement | null>(null);
  const router = useRouter();

  // Close on Escape
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  async function handleRemove() {
    setError(null);
    setSubmitting(true);
    try {
      const res = await fetch(
        `/api/orgs/${orgId}/members/${member.membershipId}`,
        {
          method: "DELETE",
          credentials: "include",
        },
      );
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as
          | { error?: string }
          | null;
        if (res.status === 403) {
          setError("You don't have permission to do that.");
        } else if (res.status === 404) {
          setError("Membership no longer exists; refreshing.");
          router.refresh();
          onDone();
        } else {
          setError(body?.error ?? `Request failed: ${res.status}`);
        }
        return;
      }
      router.refresh();
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      ref={backdropRef}
      role="dialog"
      aria-modal="true"
      aria-label="Remove member"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        if (e.target === backdropRef.current) onClose();
      }}
    >
      <div className="w-full max-w-sm rounded-lg bg-background p-6 shadow-xl">
        <h2 className="text-lg font-semibold mb-1">Remove member</h2>
        <p className="text-sm text-muted-foreground mb-4">
          Remove{" "}
          <span className="font-medium">{member.name}</span> from this
          organization? They&apos;ll lose access immediately. This is reversible
          by re-inviting.
        </p>

        {error && (
          <div
            role="alert"
            className="mb-4 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
          >
            {error}
          </div>
        )}

        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="rounded-md border border-input bg-background px-4 py-2 text-sm font-medium hover:bg-muted disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleRemove}
            disabled={submitting}
            className="rounded-md bg-destructive px-4 py-2 text-sm font-medium text-destructive-foreground hover:bg-destructive/90 disabled:opacity-50"
          >
            {submitting ? "Removing…" : "Remove"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ─── 3-dot menu ──────────────────────────────────────────────────────────────

type ModalState = "none" | "update-status" | "remove";

export function MemberRowActions({ orgId, member, isSelf }: Props) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [modal, setModal] = useState<ModalState>("none");
  const menuRef = useRef<HTMLDivElement | null>(null);

  // Close dropdown when clicking outside
  useEffect(() => {
    if (!menuOpen) return;
    function onPointerDown(e: PointerEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    }
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [menuOpen]);

  // Close dropdown on Escape
  useEffect(() => {
    if (!menuOpen) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setMenuOpen(false);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [menuOpen]);

  function openUpdateStatus() {
    setMenuOpen(false);
    setModal("update-status");
  }

  function openRemove() {
    if (isSelf) return;
    setMenuOpen(false);
    setModal("remove");
  }

  function closeModal() {
    setModal("none");
  }

  return (
    <>
      <div ref={menuRef} className="relative inline-block">
        <button
          type="button"
          aria-label="Member actions"
          aria-haspopup="true"
          aria-expanded={menuOpen}
          onClick={() => setMenuOpen((v) => !v)}
          className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          ⋮
        </button>

        {menuOpen && (
          <div
            role="menu"
            className="absolute right-0 z-20 mt-1 w-48 rounded-md border border-border bg-background shadow-lg"
          >
            <button
              role="menuitem"
              type="button"
              onClick={openUpdateStatus}
              className="w-full px-4 py-2 text-left text-sm hover:bg-muted"
            >
              Update status…
            </button>

            {isSelf ? (
              <span
                role="menuitem"
                aria-disabled="true"
                title="Use the org transfer flow to leave an org."
                className="block w-full cursor-not-allowed px-4 py-2 text-left text-sm text-muted-foreground opacity-50"
              >
                Remove
              </span>
            ) : (
              <button
                role="menuitem"
                type="button"
                onClick={openRemove}
                className="w-full px-4 py-2 text-left text-sm text-rose-600 hover:bg-rose-50"
              >
                Remove
              </button>
            )}
          </div>
        )}
      </div>

      {modal === "update-status" && (
        <UpdateStatusModal
          orgId={orgId}
          member={member}
          isSelf={isSelf}
          onClose={closeModal}
          onDone={closeModal}
        />
      )}

      {modal === "remove" && (
        <RemoveDialog
          orgId={orgId}
          member={member}
          onClose={closeModal}
          onDone={closeModal}
        />
      )}
    </>
  );
}
