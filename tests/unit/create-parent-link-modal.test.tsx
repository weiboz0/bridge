// @vitest-environment jsdom

import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";

import { CreateParentLinkModal } from "@/components/org/create-parent-link-modal";
import type { OrgStudentRow } from "@/app/(portal)/org/parent-links/page";

// Plan 084 — regression tests for the parent-link picker UX:
// 1. Small-org focus opens the suggestion list immediately.
// 2. Large-org focus does NOT open the list (preserves "type to search").
// 3. Submit button is disabled until both parent email + child are valid.

function makeStudents(n: number): OrgStudentRow[] {
  return Array.from({ length: n }, (_, i) => ({
    userId: `00000000-0000-0000-0000-${String(i + 1).padStart(12, "0")}`,
    name: `Student ${i + 1}`,
    email: `student${i + 1}@demo.edu`,
  }));
}

const ORG_ID = "f47ac10b-58cc-4372-a567-0e02b2c3d479";

beforeEach(() => {
  (globalThis as unknown as { fetch: typeof fetch | undefined }).fetch = undefined;
});

describe("CreateParentLinkModal — Plan 084 picker UX", () => {
  it("opens the suggestion list on focus for small student sets (<=8)", () => {
    const students = makeStudents(3);

    render(
      <CreateParentLinkModal
        orgId={ORG_ID}
        students={students}
        studentsError={null}
        onClose={() => {}}
        onCreated={() => {}}
      />
    );

    // Before focus, the listbox should not be visible.
    expect(screen.queryByRole("listbox", { name: /child suggestions/i })).toBeNull();

    // Focus the child input — listbox should appear immediately with all 3 students.
    const childInput = screen.getByRole("combobox");
    fireEvent.focus(childInput);

    const listbox = screen.getByRole("listbox", { name: /child suggestions/i });
    expect(listbox).toBeInTheDocument();
    expect(screen.getByText("Student 1")).toBeInTheDocument();
    expect(screen.getByText("Student 2")).toBeInTheDocument();
    expect(screen.getByText("Student 3")).toBeInTheDocument();

    // ARIA attributes track the visibility.
    expect(childInput).toHaveAttribute("aria-expanded", "true");
    expect(childInput).toHaveAttribute("aria-controls", "child-autocomplete-listbox");
  });

  it("does NOT open the list on focus for large student sets (>8)", () => {
    const students = makeStudents(20);

    render(
      <CreateParentLinkModal
        orgId={ORG_ID}
        students={students}
        studentsError={null}
        onClose={() => {}}
        onCreated={() => {}}
      />
    );

    const childInput = screen.getByRole("combobox");
    fireEvent.focus(childInput);

    // Listbox should NOT appear — user must type to search.
    expect(screen.queryByRole("listbox", { name: /child suggestions/i })).toBeNull();

    // ARIA reflects the hidden state.
    expect(childInput).toHaveAttribute("aria-expanded", "false");
    expect(childInput).not.toHaveAttribute("aria-controls");
  });

  it("disables the Create link button until both parent email and child are set", () => {
    const students = makeStudents(2);

    render(
      <CreateParentLinkModal
        orgId={ORG_ID}
        students={students}
        studentsError={null}
        onClose={() => {}}
        onCreated={() => {}}
      />
    );

    const submit = screen.getByRole("button", { name: /create link/i });

    // Initial state: both fields empty → disabled.
    expect(submit).toBeDisabled();

    // Fill parent email only → still disabled.
    const parentInput = screen.getByLabelText(/parent email/i);
    fireEvent.change(parentInput, { target: { value: "diana@demo.edu" } });
    expect(submit).toBeDisabled();

    // Pick a child — focus child input, click a suggestion. fireEvent
    // uses mousedown (the preventDefault path) then click.
    const childInput = screen.getByRole("combobox");
    fireEvent.focus(childInput);
    const option = screen.getByText("Student 1");
    act(() => {
      fireEvent.mouseDown(option);
      fireEvent.click(option);
    });

    // Now both are set → enabled.
    expect(submit).toBeEnabled();
  });
});
