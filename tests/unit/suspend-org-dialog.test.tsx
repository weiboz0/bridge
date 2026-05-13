// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SuspendOrgDialog } from "@/components/admin/suspend-org-dialog";

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
  orgId: "org-abc",
  orgName: "Riverdale School",
  open: true,
  onClose: vi.fn(),
  onSuspended: vi.fn(),
};

describe("SuspendOrgDialog", () => {
  it("returns null when open is false — no dialog rendered", () => {
    const { container } = render(<SuspendOrgDialog {...defaultProps} open={false} />);
    expect(container.firstChild).toBeNull();
    expect(screen.queryByText(/Suspend organization/i)).toBeNull();
  });

  it("Suspend button is disabled initially and stays disabled while typed text does not match", () => {
    render(<SuspendOrgDialog {...defaultProps} />);
    const btn = screen.getByRole("button", { name: /Suspend organization/i });
    expect(btn).toBeDisabled();

    const input = screen.getByLabelText(/Type organization name to confirm/i);
    fireEvent.change(input, { target: { value: "riverdale school" } }); // wrong case
    expect(btn).toBeDisabled();

    fireEvent.change(input, { target: { value: "Riverdale" } }); // partial match
    expect(btn).toBeDisabled();
  });

  it("enables Suspend button only when typed text exactly matches orgName", () => {
    render(<SuspendOrgDialog {...defaultProps} />);
    const btn = screen.getByRole("button", { name: /Suspend organization/i });
    const input = screen.getByLabelText(/Type organization name to confirm/i);

    fireEvent.change(input, { target: { value: "Riverdale School" } });
    expect(btn).not.toBeDisabled();
  });

  it("issues PATCH, calls onSuspended + onClose on 2xx success", async () => {
    const fetchMock = mockFetch(204);
    const onClose = vi.fn();
    const onSuspended = vi.fn();

    render(
      <SuspendOrgDialog
        {...defaultProps}
        onClose={onClose}
        onSuspended={onSuspended}
      />
    );

    const input = screen.getByLabelText(/Type organization name to confirm/i);
    fireEvent.change(input, { target: { value: "Riverdale School" } });

    const btn = screen.getByRole("button", { name: /Suspend organization/i });
    fireEvent.click(btn);

    await waitFor(() => expect(onSuspended).toHaveBeenCalledTimes(1));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/orgs/org-abc",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ status: "suspended" }),
      })
    );
  });

  it("shows body.error inline and does NOT call onSuspended/onClose on non-2xx", async () => {
    mockFetch(422, { error: "Cannot suspend: active billing" });
    const onClose = vi.fn();
    const onSuspended = vi.fn();

    render(
      <SuspendOrgDialog
        {...defaultProps}
        onClose={onClose}
        onSuspended={onSuspended}
      />
    );

    const input = screen.getByLabelText(/Type organization name to confirm/i);
    fireEvent.change(input, { target: { value: "Riverdale School" } });

    const btn = screen.getByRole("button", { name: /Suspend organization/i });
    fireEvent.click(btn);

    await waitFor(() =>
      expect(screen.getByText(/Cannot suspend: active billing/i)).toBeInTheDocument()
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
      <SuspendOrgDialog
        {...defaultProps}
        onClose={onClose}
        onSuspended={onSuspended}
      />
    );

    const input = screen.getByLabelText(/Type organization name to confirm/i);
    fireEvent.change(input, { target: { value: "Riverdale School" } });
    fireEvent.click(screen.getByRole("button", { name: /Suspend organization/i }));

    await waitFor(() =>
      expect(screen.getByText(/Network is unreachable/i)).toBeInTheDocument()
    );
    expect(onSuspended).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    // The error node is announced via role="alert" for screen readers.
    expect(screen.getByRole("alert")).toHaveTextContent(/Network is unreachable/i);
  });

  it("resets typed input + error when reopened", () => {
    const { rerender } = render(<SuspendOrgDialog {...defaultProps} />);

    const input = screen.getByLabelText(/Type organization name to confirm/i) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Riverdale School" } });
    expect(input.value).toBe("Riverdale School");

    // Close the dialog.
    rerender(<SuspendOrgDialog {...defaultProps} open={false} />);
    expect(screen.queryByLabelText(/Type organization name to confirm/i)).toBeNull();

    // Reopen — the input should be cleared.
    rerender(<SuspendOrgDialog {...defaultProps} open={true} />);
    const reopened = screen.getByLabelText(/Type organization name to confirm/i) as HTMLInputElement;
    expect(reopened.value).toBe("");
  });

  it("matches the gate even when orgName has trailing whitespace (symmetric trim)", () => {
    render(<SuspendOrgDialog {...defaultProps} orgName="Riverdale School  " />);
    const input = screen.getByLabelText(/Type organization name to confirm/i);
    fireEvent.change(input, { target: { value: "Riverdale School" } });
    expect(screen.getByRole("button", { name: /Suspend organization/i })).not.toBeDisabled();
  });
});
