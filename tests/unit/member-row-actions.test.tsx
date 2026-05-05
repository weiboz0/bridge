// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn(), back: vi.fn() }),
}));

import { MemberRowActions } from "@/components/org/member-row-actions";

const activeMember = {
  membershipId: "m1",
  userId: "u1",
  name: "Alice Teacher",
  email: "alice@demo.edu",
  status: "active",
};

const suspendedMember = {
  membershipId: "m2",
  userId: "u2",
  name: "Bob Student",
  email: "bob@demo.edu",
  status: "suspended",
};

function openMenu(container: HTMLElement) {
  const btn = container.querySelector("button[aria-label='Member actions']");
  if (!btn) throw new Error("Could not find menu trigger button");
  fireEvent.click(btn);
}

describe("MemberRowActions — isSelf=false", () => {
  it("opens the menu on click", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={activeMember} isSelf={false} />,
    );
    openMenu(container);
    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  it("shows both Update status and Remove options", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={activeMember} isSelf={false} />,
    );
    openMenu(container);
    expect(screen.getByText("Update status…")).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Remove" })).toBeInTheDocument();
  });

  it("Remove option is a button (not disabled)", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={activeMember} isSelf={false} />,
    );
    openMenu(container);
    const removeItem = screen.getByRole("menuitem", { name: "Remove" });
    expect(removeItem.tagName).toBe("BUTTON");
    expect(removeItem).not.toHaveAttribute("aria-disabled");
  });

  it("opens Update-status modal with all three options enabled", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={activeMember} isSelf={false} />,
    );
    openMenu(container);
    fireEvent.click(screen.getByText("Update status…"));
    // Modal should be open
    expect(screen.getByRole("dialog", { name: /update member status/i })).toBeInTheDocument();
    // Suspended option should NOT be disabled for non-self
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    const options = Array.from(select.options);
    const suspendedOpt = options.find((o) => o.value === "suspended");
    expect(suspendedOpt).toBeDefined();
    expect(suspendedOpt?.disabled).toBe(false);
  });

  it("opens Remove confirm dialog", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={activeMember} isSelf={false} />,
    );
    openMenu(container);
    fireEvent.click(screen.getByRole("menuitem", { name: "Remove" }));
    expect(screen.getByRole("dialog", { name: /remove member/i })).toBeInTheDocument();
    expect(screen.getByText(/Alice Teacher/)).toBeInTheDocument();
  });
});

describe("MemberRowActions — isSelf=true", () => {
  it("shows the menu when triggered", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={activeMember} isSelf={true} />,
    );
    openMenu(container);
    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  it("Remove option is disabled (aria-disabled=true, not a button)", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={activeMember} isSelf={true} />,
    );
    openMenu(container);
    const removeItem = screen.getByRole("menuitem", { name: /remove/i });
    expect(removeItem).toHaveAttribute("aria-disabled", "true");
    expect(removeItem.tagName).not.toBe("BUTTON");
  });

  it("Remove option has tooltip about org transfer flow", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={activeMember} isSelf={true} />,
    );
    openMenu(container);
    const removeItem = screen.getByRole("menuitem", { name: /remove/i });
    expect(removeItem).toHaveAttribute("title", expect.stringContaining("org transfer flow"));
  });

  it("suspended option is disabled in Update-status modal", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={activeMember} isSelf={true} />,
    );
    openMenu(container);
    fireEvent.click(screen.getByText("Update status…"));
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    const options = Array.from(select.options);
    const suspendedOpt = options.find((o) => o.value === "suspended");
    expect(suspendedOpt).toBeDefined();
    expect(suspendedOpt?.disabled).toBe(true);
  });

  it("clicking disabled Remove in menu does NOT open the remove dialog", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={activeMember} isSelf={true} />,
    );
    openMenu(container);
    // Click the disabled item — no dialog should appear
    const removeItem = screen.getByRole("menuitem", { name: /remove/i });
    fireEvent.click(removeItem);
    expect(
      screen.queryByRole("dialog", { name: /remove member/i }),
    ).not.toBeInTheDocument();
  });
});

describe("MemberRowActions — suspended member", () => {
  it("shows suspended status badge properly in Update-status modal", () => {
    const { container } = render(
      <MemberRowActions orgId="org1" member={suspendedMember} isSelf={false} />,
    );
    openMenu(container);
    fireEvent.click(screen.getByText("Update status…"));
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    // Should default to the member's current status
    expect(select.value).toBe("suspended");
  });
});
