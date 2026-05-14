// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { OrgEditDialog } from "@/components/admin/org-edit-dialog";

beforeEach(() => {
  (globalThis as unknown as { fetch: typeof fetch | undefined }).fetch = undefined;
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

const BASE_ORG = {
  id: "org-001",
  name: "Riverside Academy",
  contactName: "Jane Smith",
  contactEmail: "jane@riverside.edu",
};

const defaultProps = {
  org: BASE_ORG,
  open: true,
  onClose: vi.fn(),
  onSaved: vi.fn(),
};

beforeEach(() => {
  defaultProps.onClose.mockReset();
  defaultProps.onSaved.mockReset();
});

describe("OrgEditDialog", () => {
  it("Save disabled when all three fields are unchanged from initial values", () => {
    render(<OrgEditDialog {...defaultProps} />);
    const saveBtn = screen.getByRole("button", { name: "Save" });
    expect(saveBtn).toBeDisabled();
  });

  it("Save disabled when name field is empty after trim", () => {
    render(<OrgEditDialog {...defaultProps} />);

    const nameInput = screen.getByLabelText(/^Name/i);
    fireEvent.change(nameInput, { target: { value: "   " } });

    const saveBtn = screen.getByRole("button", { name: "Save" });
    expect(saveBtn).toBeDisabled();
  });

  it("Save disabled when contact name field is empty after trim", () => {
    render(<OrgEditDialog {...defaultProps} />);

    const nameInput = screen.getByLabelText(/^Name/i);
    fireEvent.change(nameInput, { target: { value: "New Name" } });

    const contactNameInput = screen.getByLabelText(/Contact name/i);
    fireEvent.change(contactNameInput, { target: { value: "   " } });

    const saveBtn = screen.getByRole("button", { name: "Save" });
    expect(saveBtn).toBeDisabled();
  });

  it("Save disabled when contact email field is empty after trim", () => {
    render(<OrgEditDialog {...defaultProps} />);

    const nameInput = screen.getByLabelText(/^Name/i);
    fireEvent.change(nameInput, { target: { value: "New Name" } });

    const emailInput = screen.getByLabelText(/Contact email/i);
    fireEvent.change(emailInput, { target: { value: "" } });

    const saveBtn = screen.getByRole("button", { name: "Save" });
    expect(saveBtn).toBeDisabled();
  });

  it("Save enabled when any field differs from initial (trimmed)", () => {
    render(<OrgEditDialog {...defaultProps} />);

    const nameInput = screen.getByLabelText(/^Name/i);
    fireEvent.change(nameInput, { target: { value: "New Academy Name" } });

    const saveBtn = screen.getByRole("button", { name: "Save" });
    expect(saveBtn).not.toBeDisabled();
  });

  it("successful PATCH calls onSaved + onClose; surfaces no error", async () => {
    const fetchMock = mockFetch(200, { ...BASE_ORG, name: "New Academy Name" });
    const onClose = vi.fn();
    const onSaved = vi.fn();

    render(<OrgEditDialog {...defaultProps} onClose={onClose} onSaved={onSaved} />);

    const nameInput = screen.getByLabelText(/^Name/i);
    fireEvent.change(nameInput, { target: { value: "New Academy Name" } });

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/orgs/org-001/details",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({
          name: "New Academy Name",
          contactName: "Jane Smith",
          contactEmail: "jane@riverside.edu",
        }),
      })
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("non-2xx surfaces error inline with role='alert'", async () => {
    mockFetch(400, { error: "Name already taken" });
    const onClose = vi.fn();
    const onSaved = vi.fn();

    render(<OrgEditDialog {...defaultProps} onClose={onClose} onSaved={onSaved} />);

    const nameInput = screen.getByLabelText(/^Name/i);
    fireEvent.change(nameInput, { target: { value: "Taken Name" } });

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Name already taken")
    );
    expect(onSaved).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("network rejection surfaces error inline with role='alert'", async () => {
    (globalThis as unknown as { fetch: typeof fetch }).fetch = vi.fn(async () => {
      throw new Error("Network is unreachable");
    }) as unknown as typeof fetch;
    const onClose = vi.fn();
    const onSaved = vi.fn();

    render(<OrgEditDialog {...defaultProps} onClose={onClose} onSaved={onSaved} />);

    const nameInput = screen.getByLabelText(/^Name/i);
    fireEvent.change(nameInput, { target: { value: "Some Name" } });

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Network is unreachable")
    );
    expect(onSaved).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("reopen resets fields to initial org values", () => {
    const { rerender } = render(<OrgEditDialog {...defaultProps} />);

    const nameInput = screen.getByLabelText(/^Name/i) as HTMLInputElement;
    fireEvent.change(nameInput, { target: { value: "Modified Name" } });
    expect(nameInput.value).toBe("Modified Name");

    // Close the dialog.
    rerender(<OrgEditDialog {...defaultProps} open={false} />);
    expect(screen.queryByLabelText(/^Name/i)).toBeNull();

    // Reopen — fields should be reset to initial org values.
    rerender(<OrgEditDialog {...defaultProps} open={true} />);
    const reopenedInput = screen.getByLabelText(/^Name/i) as HTMLInputElement;
    expect(reopenedInput.value).toBe("Riverside Academy");
  });
});
