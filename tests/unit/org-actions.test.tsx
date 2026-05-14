// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";

const mockRefresh = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: mockRefresh }),
}));

vi.mock("next/link", () => ({
  default: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => (
    <a href={href} className={className}>{children}</a>
  ),
}));

beforeEach(() => {
  (globalThis as unknown as { fetch: typeof fetch | undefined }).fetch = undefined;
  mockRefresh.mockReset();
});

function mockFetch(status: number, body?: unknown) {
  (globalThis as unknown as { fetch: typeof fetch }).fetch = vi.fn(async () =>
    new Response(body == null ? null : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    })
  ) as unknown as typeof fetch;
  return (globalThis as unknown as { fetch: ReturnType<typeof vi.fn> }).fetch;
}

import { OrgActions } from "@/components/admin/org-actions";

const BASE_PROPS = {
  orgId: "org-001",
  orgName: "Riverside Academy",
  status: "active" as const,
  contactName: "Jane Smith",
  contactEmail: "jane@riverside.edu",
};

function openMenu() {
  const btn = screen.getByRole("button", { name: /Actions/i });
  fireEvent.click(btn);
}

describe("OrgActions — pending row", () => {
  it("shows inline Approve button and dropdown with View details only", () => {
    render(<OrgActions {...BASE_PROPS} status="pending" />);

    expect(screen.getByRole("button", { name: "Approve" })).toBeInTheDocument();
    openMenu();
    expect(screen.getByText("View details")).toBeInTheDocument();
    // Active-only items should not appear
    expect(screen.queryByText("Edit organization…")).toBeNull();
    expect(screen.queryByText("Suspend organization…")).toBeNull();
    expect(screen.queryByText("Reactivate organization")).toBeNull();
  });
});

describe("OrgActions — active row", () => {
  it("has no inline button; dropdown has View details, Edit organization…, Suspend organization…", () => {
    render(<OrgActions {...BASE_PROPS} status="active" />);

    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    openMenu();
    expect(screen.getByText("View details")).toBeInTheDocument();
    expect(screen.getByText("Edit organization…")).toBeInTheDocument();
    expect(screen.getByText("Suspend organization…")).toBeInTheDocument();
    expect(screen.queryByText("Reactivate organization")).toBeNull();
  });
});

describe("OrgActions — suspended row", () => {
  it("dropdown has View details and Reactivate organization; no Approve or Edit", () => {
    render(<OrgActions {...BASE_PROPS} status="suspended" />);

    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    openMenu();
    expect(screen.getByText("View details")).toBeInTheDocument();
    expect(screen.getByText("Reactivate organization")).toBeInTheDocument();
    expect(screen.queryByText("Edit organization…")).toBeNull();
    expect(screen.queryByText("Suspend organization…")).toBeNull();
  });
});

describe("OrgActions — Approve action (pending)", () => {
  it("clicking Approve opens ConfirmDialog; clicking its Confirm fires PATCH with {status: active}", async () => {
    const fetchMock = mockFetch(200, { id: "org-001", status: "active" });
    render(<OrgActions {...BASE_PROPS} status="pending" />);

    // Click the inline Approve button.
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    // ConfirmDialog should open.
    const dialog = screen.getByRole("dialog");
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText(/Activate Riverside Academy/i)).toBeInTheDocument();

    // Click the Confirm button inside the dialog (not the inline Approve button).
    const dialogEl = screen.getByRole("dialog");
    const dialogConfirm = within(dialogEl).getByRole("button", { name: "Approve" });
    fireEvent.click(dialogConfirm);

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/admin/orgs/org-001",
        expect.objectContaining({
          method: "PATCH",
          body: JSON.stringify({ status: "active" }),
        })
      )
    );
    await waitFor(() => expect(mockRefresh).toHaveBeenCalledTimes(1));
  });
});

describe("OrgActions — Reactivate action (suspended)", () => {
  it("clicking Reactivate opens ConfirmDialog; firing Confirm fires PATCH with {status: active}", async () => {
    const fetchMock = mockFetch(200, { id: "org-001", status: "active" });
    render(<OrgActions {...BASE_PROPS} status="suspended" />);

    openMenu();
    fireEvent.click(screen.getByText("Reactivate organization"));

    const dialog = screen.getByRole("dialog");
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText(/Reactivate Riverside Academy/i)).toBeInTheDocument();

    // Click the Confirm button inside the dialog.
    const dialogConfirm = screen.getByRole("button", { name: "Reactivate" });
    fireEvent.click(dialogConfirm);

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/admin/orgs/org-001",
        expect.objectContaining({
          method: "PATCH",
          body: JSON.stringify({ status: "active" }),
        })
      )
    );
    await waitFor(() => expect(mockRefresh).toHaveBeenCalledTimes(1));
  });
});

describe("OrgActions — Edit action (active)", () => {
  it("clicking Edit organization… opens OrgEditDialog", () => {
    render(<OrgActions {...BASE_PROPS} status="active" />);

    openMenu();
    fireEvent.click(screen.getByText("Edit organization…"));

    // OrgEditDialog should be rendered (role="dialog" with the edit title).
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Edit organization")).toBeInTheDocument();
  });
});
