// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SuspendUserDialog } from "@/components/admin/suspend-user-dialog";

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

const defaultProps = {
  userId: "user-abc",
  userName: "Alice Smith",
  open: true,
  onClose: vi.fn(),
  onSuspended: vi.fn(),
};

describe("SuspendUserDialog", () => {
  it("returns null when open is false — no dialog rendered", () => {
    const { container } = render(<SuspendUserDialog {...defaultProps} open={false} />);
    expect(container.firstChild).toBeNull();
    expect(screen.queryByText(/Suspend user account/i)).toBeNull();
  });

  it("Suspend button is disabled initially and stays disabled while typed text does not match", () => {
    render(<SuspendUserDialog {...defaultProps} />);
    const btn = screen.getByRole("button", { name: /Suspend account/i });
    expect(btn).toBeDisabled();

    const input = screen.getByLabelText(/Type user name to confirm/i);
    fireEvent.change(input, { target: { value: "alice smith" } }); // wrong case
    expect(btn).toBeDisabled();

    fireEvent.change(input, { target: { value: "Alice" } }); // partial match
    expect(btn).toBeDisabled();
  });

  it("enables Suspend button only when typed text exactly matches userName", () => {
    render(<SuspendUserDialog {...defaultProps} />);
    const btn = screen.getByRole("button", { name: /Suspend account/i });
    const input = screen.getByLabelText(/Type user name to confirm/i);

    fireEvent.change(input, { target: { value: "Alice Smith" } });
    expect(btn).not.toBeDisabled();
  });

  it("issues PATCH, calls onSuspended + onClose on 2xx success", async () => {
    const fetchMock = mockFetch(200, { id: "user-abc", status: "suspended" });
    const onClose = vi.fn();
    const onSuspended = vi.fn();

    render(
      <SuspendUserDialog
        {...defaultProps}
        onClose={onClose}
        onSuspended={onSuspended}
      />
    );

    const input = screen.getByLabelText(/Type user name to confirm/i);
    fireEvent.change(input, { target: { value: "Alice Smith" } });

    const btn = screen.getByRole("button", { name: /Suspend account/i });
    fireEvent.click(btn);

    await waitFor(() => expect(onSuspended).toHaveBeenCalledTimes(1));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/users/user-abc/status",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ status: "suspended" }),
      })
    );
  });

  it("shows body.error inline and does NOT call onSuspended/onClose on non-2xx", async () => {
    mockFetch(422, { error: "Cannot suspend: self-suspend not allowed" });
    const onClose = vi.fn();
    const onSuspended = vi.fn();

    render(
      <SuspendUserDialog
        {...defaultProps}
        onClose={onClose}
        onSuspended={onSuspended}
      />
    );

    const input = screen.getByLabelText(/Type user name to confirm/i);
    fireEvent.change(input, { target: { value: "Alice Smith" } });

    const btn = screen.getByRole("button", { name: /Suspend account/i });
    fireEvent.click(btn);

    await waitFor(() =>
      expect(screen.getByText(/Cannot suspend: self-suspend not allowed/i)).toBeInTheDocument()
    );
    expect(onSuspended).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("surfaces a network error inline when fetch rejects", async () => {
    (globalThis as unknown as { fetch: typeof fetch }).fetch = vi.fn(
      async () => {
        throw new Error("Network is unreachable");
      }
    ) as unknown as typeof fetch;
    const onClose = vi.fn();
    const onSuspended = vi.fn();

    render(
      <SuspendUserDialog
        {...defaultProps}
        onClose={onClose}
        onSuspended={onSuspended}
      />
    );

    const input = screen.getByLabelText(/Type user name to confirm/i);
    fireEvent.change(input, { target: { value: "Alice Smith" } });
    fireEvent.click(screen.getByRole("button", { name: /Suspend account/i }));

    await waitFor(() =>
      expect(screen.getByText(/Network is unreachable/i)).toBeInTheDocument()
    );
    expect(onSuspended).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    // The error node is announced via role="alert" for screen readers.
    expect(screen.getByRole("alert")).toHaveTextContent(/Network is unreachable/i);
  });

  it("resets typed input + error when reopened", () => {
    const { rerender } = render(<SuspendUserDialog {...defaultProps} />);

    const input = screen.getByLabelText(/Type user name to confirm/i) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Alice Smith" } });
    expect(input.value).toBe("Alice Smith");

    // Close the dialog.
    rerender(<SuspendUserDialog {...defaultProps} open={false} />);
    expect(screen.queryByLabelText(/Type user name to confirm/i)).toBeNull();

    // Reopen — the input should be cleared.
    rerender(<SuspendUserDialog {...defaultProps} open={true} />);
    const reopened = screen.getByLabelText(/Type user name to confirm/i) as HTMLInputElement;
    expect(reopened.value).toBe("");
  });

  it("matches the gate even when userName has trailing whitespace (symmetric trim)", () => {
    render(<SuspendUserDialog {...defaultProps} userName="Alice Smith  " />);
    const input = screen.getByLabelText(/Type user name to confirm/i);
    fireEvent.change(input, { target: { value: "Alice Smith" } });
    expect(screen.getByRole("button", { name: /Suspend account/i })).not.toBeDisabled();
  });
});
