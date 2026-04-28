// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const pushMock = vi.fn();
const searchParams = new URLSearchParams();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
  usePathname: () => "/org/teachers",
  useSearchParams: () => searchParams,
}));

import { OrgSwitcher } from "@/components/portal/org-switcher";

beforeEach(() => {
  pushMock.mockReset();
});

describe("OrgSwitcher (plan 043 phase 3)", () => {
  it("returns null with fewer than 2 options", () => {
    const { container } = render(<OrgSwitcher options={[]} />);
    expect(container).toBeEmptyDOMElement();

    const { container: c2 } = render(
      <OrgSwitcher options={[{ orgId: "a", orgName: "Org A" }]} />
    );
    expect(c2).toBeEmptyDOMElement();
  });

  it("renders a select with each org as an option", () => {
    render(
      <OrgSwitcher
        options={[
          { orgId: "a", orgName: "Org A" },
          { orgId: "b", orgName: "Org B" },
        ]}
      />
    );
    const select = screen.getByRole("combobox", { name: /organization/i });
    expect(select).toBeInTheDocument();
    const options = screen.getAllByRole("option");
    expect(options).toHaveLength(2);
    expect(options[0]).toHaveTextContent("Org A");
    expect(options[1]).toHaveTextContent("Org B");
  });

  it("changing the selection pushes ?orgId=<new>", () => {
    render(
      <OrgSwitcher
        options={[
          { orgId: "a", orgName: "Org A" },
          { orgId: "b", orgName: "Org B" },
        ]}
      />
    );
    const select = screen.getByRole("combobox", { name: /organization/i });
    fireEvent.change(select, { target: { value: "b" } });
    expect(pushMock).toHaveBeenCalledTimes(1);
    expect(pushMock.mock.calls[0][0]).toBe("/org/teachers?orgId=b");
  });
});
