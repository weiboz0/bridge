"use client";

import { useState, useEffect, useCallback, useMemo } from "react";

// Plan 067 — section-expansion state for the sectioned sidebar.
//
// The sidebar shows ALL of a user's accessible portal sections (one
// per UserRole) as collapsible groups. The active section (matching
// the current URL) is auto-expanded; the others are collapsed unless
// the user has manually toggled them open.
//
// Manual toggles persist in localStorage under `bridge.sidebar.expanded`
// as a `Record<sectionKey, boolean>`. The key shape is
// `${role}:${orgId ?? "none"}` so two `org_admin` sections at different
// orgs have independent toggles.

export const SIDEBAR_EXPANDED_STORAGE_KEY = "bridge.sidebar.expanded";

export function sectionKey(role: string, orgId?: string | null): string {
  return `${role}:${orgId ?? "none"}`;
}

export function useSidebarSections(activeKey: string | null) {
  // Map of sectionKey → user's manual override (true = force-open,
  // false = force-collapsed). Keys not in the map fall through to
  // the default (active = open, others = closed).
  const [overrides, setOverrides] = useState<Record<string, boolean>>({});

  useEffect(() => {
    try {
      const raw = localStorage.getItem(SIDEBAR_EXPANDED_STORAGE_KEY);
      if (!raw) return;
      const parsed = JSON.parse(raw) as Record<string, boolean>;
      if (parsed && typeof parsed === "object") {
        setOverrides(parsed);
      }
    } catch {
      // Ignore quota/parse errors; defaults stand.
    }
  }, []);

  const isExpanded = useCallback(
    (key: string) => {
      if (key in overrides) return overrides[key];
      return key === activeKey;
    },
    [overrides, activeKey]
  );

  const toggle = useCallback(
    (key: string) => {
      setOverrides((prev) => {
        const currentlyExpanded = key in prev ? prev[key] : key === activeKey;
        const next = { ...prev, [key]: !currentlyExpanded };
        try {
          localStorage.setItem(SIDEBAR_EXPANDED_STORAGE_KEY, JSON.stringify(next));
        } catch {
          // Ignore quota errors.
        }
        return next;
      });
    },
    [activeKey]
  );

  return useMemo(() => ({ isExpanded, toggle }), [isExpanded, toggle]);
}
