// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const pushMock = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}));

import { RoleSwitcher } from "@/components/portal/role-switcher";
import type { UserRole } from "@/lib/portal/types";

beforeEach(() => {
  pushMock.mockReset();
});

describe("RoleSwitcher (plan 043 phase 3.4)", () => {
  it("returns null when only one role", () => {
    const { container } = render(
      <RoleSwitcher
        roles={[{ role: "teacher" } as UserRole]}
        currentRole="teacher"
        collapsed={false}
      />
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("two org_admin roles in different orgs render without React key warnings", () => {
    // Suppress to detect — react logs a warning to console.error when
    // duplicate keys appear in the same array.
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const roles: UserRole[] = [
      { role: "org_admin", orgId: "org-a", orgName: "Org A" },
      { role: "org_admin", orgId: "org-b", orgName: "Org B" },
    ];
    render(<RoleSwitcher roles={roles} currentRole="org_admin" collapsed={false} />);

    const dupKeyMessage = errorSpy.mock.calls.find((c) =>
      String(c[0]).includes("Encountered two children with the same key")
    );
    expect(dupKeyMessage).toBeUndefined();
    errorSpy.mockRestore();
  });

  it("clicking an org-scoped role pushes /<portal>?orgId=<id>", () => {
    const roles: UserRole[] = [
      { role: "teacher" },
      { role: "org_admin", orgId: "org-a", orgName: "Org A" },
    ];
    render(<RoleSwitcher roles={roles} currentRole="teacher" collapsed={false} />);

    // The Org Admin button text includes the org name; match on a stable
    // attribute instead.
    const orgAdminButton = screen
      .getAllByRole("button")
      .find((b) => b.textContent?.includes("Org Admin"));
    expect(orgAdminButton).toBeTruthy();
    fireEvent.click(orgAdminButton!);
    expect(pushMock).toHaveBeenCalledWith("/org?orgId=org-a");
  });

  it("clicking a non-org-scoped role pushes the bare portal path", () => {
    const roles: UserRole[] = [
      { role: "teacher" },
      { role: "student" },
    ];
    render(<RoleSwitcher roles={roles} currentRole="teacher" collapsed={false} />);

    const studentButton = screen.getByRole("button", { name: /Student/i });
    fireEvent.click(studentButton);
    expect(pushMock).toHaveBeenCalledWith("/student");
  });
});
