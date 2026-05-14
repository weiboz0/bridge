// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const mockRefresh = vi.fn();
const mockPush = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush, refresh: mockRefresh, back: vi.fn() }),
}));

vi.mock("next/link", () => ({
  default: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => (
    <a href={href} className={className}>{children}</a>
  ),
}));

beforeEach(() => {
  (globalThis as unknown as { fetch: typeof fetch | undefined }).fetch = undefined;
  mockRefresh.mockReset();
  mockPush.mockReset();
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

import { UserActions } from "@/components/admin/user-actions";

const BASE_PROPS = {
  userId: "user-001",
  userName: "Alice Teacher",
  status: "active" as const,
  isPlatformAdmin: false,
  isSelf: false,
};

function openMenu() {
  const btn = screen.getByRole("button", { name: /Actions/i });
  fireEvent.click(btn);
}

describe("UserActions — active row (not self)", () => {
  it("shows View details, Login as, Toggle platform-admin, and Suspend items", () => {
    render(<UserActions {...BASE_PROPS} />);
    openMenu();

    expect(screen.getByText("View details")).toBeInTheDocument();
    expect(screen.getByText(/Login as Alice/i)).toBeInTheDocument();
    expect(screen.getByText(/Make Alice a platform admin/i)).toBeInTheDocument();
    expect(screen.getByText("Suspend account…")).toBeInTheDocument();
  });

  it("does NOT show Reactivate item for active user", () => {
    render(<UserActions {...BASE_PROPS} />);
    openMenu();

    expect(screen.queryByText("Reactivate account")).toBeNull();
  });
});

describe("UserActions — suspended row (not self)", () => {
  it("shows View details, Toggle platform-admin, and Reactivate items", () => {
    render(<UserActions {...BASE_PROPS} status="suspended" />);
    openMenu();

    expect(screen.getByText("View details")).toBeInTheDocument();
    expect(screen.getByText(/platform admin/i)).toBeInTheDocument();
    expect(screen.getByText("Reactivate account")).toBeInTheDocument();
  });

  it("does NOT show Login as or Suspend for suspended user", () => {
    render(<UserActions {...BASE_PROPS} status="suspended" />);
    openMenu();

    expect(screen.queryByText(/Login as/i)).toBeNull();
    expect(screen.queryByText("Suspend account…")).toBeNull();
  });
});

describe("UserActions — self row (isSelf=true)", () => {
  it("shows only View details; hides Login as, Suspend, Toggle platform-admin", () => {
    render(<UserActions {...BASE_PROPS} isSelf={true} />);
    openMenu();

    expect(screen.getByText("View details")).toBeInTheDocument();
    expect(screen.queryByText(/Login as/i)).toBeNull();
    expect(screen.queryByText("Suspend account…")).toBeNull();
    expect(screen.queryByText(/platform admin/i)).toBeNull();
    expect(screen.queryByText("Reactivate account")).toBeNull();
  });
});

describe("UserActions — Reactivate action (via ConfirmDialog)", () => {
  it("opens ConfirmDialog (not window.confirm) and fires PATCH /status with {status:active} + calls router.refresh", async () => {
    const fetchMock = mockFetch(200, { id: "user-001", status: "active" });
    render(<UserActions {...BASE_PROPS} status="suspended" />);
    openMenu();

    fireEvent.click(screen.getByText("Reactivate account"));

    // ConfirmDialog should open — no window.confirm.
    const dialog = screen.getByRole("dialog");
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText(/Alice Teacher will be able to sign in again/i)).toBeInTheDocument();

    // Click the Confirm button in the dialog.
    fireEvent.click(screen.getByRole("button", { name: "Reactivate" }));

    await waitFor(() => expect(mockRefresh).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/users/user-001/status",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ status: "active" }),
      })
    );
  });
});

describe("UserActions — Toggle platform-admin action (via ConfirmDialog)", () => {
  it("opens ConfirmDialog (not window.confirm) and fires PATCH /platform-admin with {isPlatformAdmin: true}", async () => {
    const fetchMock = mockFetch(200, { id: "user-001", isPlatformAdmin: true });
    render(<UserActions {...BASE_PROPS} isPlatformAdmin={false} />);
    openMenu();

    fireEvent.click(screen.getByText(/Make Alice a platform admin/i));

    // ConfirmDialog should open — no window.confirm.
    const dialog = screen.getByRole("dialog");
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText(/Grant platform-admin role/i)).toBeInTheDocument();
    expect(screen.getByText(/full access to \/admin/i)).toBeInTheDocument();

    // Click the Grant button in the dialog.
    fireEvent.click(screen.getByRole("button", { name: "Grant" }));

    await waitFor(() => expect(mockRefresh).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/users/user-001/platform-admin",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ isPlatformAdmin: true }),
      })
    );
  });

  it("uses Remove platform-admin role copy and destructive confirm when user is already a platform admin", () => {
    render(<UserActions {...BASE_PROPS} isPlatformAdmin={true} />);
    openMenu();

    // Click the menu item.
    fireEvent.click(screen.getByText(/Remove Alice/i));

    // ConfirmDialog opens with remove-admin copy.
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Remove platform-admin role")).toBeInTheDocument();
    expect(screen.getByText(/lose access to \/admin/i)).toBeInTheDocument();

    // The Remove button should use the destructive variant (bg-destructive/10).
    const removeBtn = screen.getByRole("button", { name: "Remove" });
    expect(removeBtn.className).toMatch(/bg-destructive/);
  });

  it("uses Grant platform-admin role copy (non-destructive) when user is not an admin", () => {
    render(<UserActions {...BASE_PROPS} isPlatformAdmin={false} />);
    openMenu();

    fireEvent.click(screen.getByText(/Make Alice a platform admin/i));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Grant platform-admin role")).toBeInTheDocument();

    // The Grant button should use the primary variant (not bg-destructive/10).
    const grantBtn = screen.getByRole("button", { name: "Grant" });
    expect(grantBtn.className).toMatch(/bg-primary/);
    expect(grantBtn.className).not.toMatch(/bg-destructive/);
  });
});
