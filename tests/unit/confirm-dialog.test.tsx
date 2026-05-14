// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";

const defaultProps = {
  open: true,
  onClose: vi.fn(),
  onConfirm: vi.fn(),
  title: "Confirm action",
  body: "Are you sure you want to do this?",
};

beforeEach(() => {
  defaultProps.onClose.mockReset();
  defaultProps.onConfirm.mockReset();
});

describe("ConfirmDialog", () => {
  it("returns null when open is false", () => {
    const { container } = render(<ConfirmDialog {...defaultProps} open={false} />);
    expect(container.firstChild).toBeNull();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("renders title and body when open", () => {
    render(<ConfirmDialog {...defaultProps} />);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Confirm action")).toBeInTheDocument();
    expect(screen.getByText("Are you sure you want to do this?")).toBeInTheDocument();
  });

  it("Cancel button calls onClose", () => {
    render(<ConfirmDialog {...defaultProps} />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(defaultProps.onClose).toHaveBeenCalledTimes(1);
  });

  it("Confirm fires onConfirm; on resolve, onClose is called automatically (auto-close contract)", async () => {
    const onConfirm = vi.fn().mockResolvedValueOnce(undefined);
    const onClose = vi.fn();
    render(<ConfirmDialog {...defaultProps} onConfirm={onConfirm} onClose={onClose} />);

    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("Confirm error (reject) surfaces inline with role='alert'; onClose NOT called", async () => {
    const onConfirm = vi.fn().mockRejectedValueOnce(new Error("Something went wrong"));
    const onClose = vi.fn();
    render(<ConfirmDialog {...defaultProps} onConfirm={onConfirm} onClose={onClose} />);

    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Something went wrong")
    );
    expect(onClose).not.toHaveBeenCalled();
  });

  it("resets error on reopen — the alert message disappears", async () => {
    const onConfirm = vi.fn().mockRejectedValueOnce(new Error("Error message"));
    const onClose = vi.fn();

    const { rerender } = render(
      <ConfirmDialog {...defaultProps} onConfirm={onConfirm} onClose={onClose} />
    );

    // Trigger an error.
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());

    // Close and reopen.
    rerender(
      <ConfirmDialog {...defaultProps} open={false} onConfirm={onConfirm} onClose={onClose} />
    );
    rerender(
      <ConfirmDialog {...defaultProps} open={true} onConfirm={onConfirm} onClose={onClose} />
    );

    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("Escape is suppressed while submitting — onClose not called until submit resolves", async () => {
    let resolveConfirm: () => void;
    const pendingPromise = new Promise<void>((resolve) => {
      resolveConfirm = resolve;
    });
    const onConfirm = vi.fn().mockReturnValueOnce(pendingPromise);
    const onClose = vi.fn();

    render(<ConfirmDialog {...defaultProps} onConfirm={onConfirm} onClose={onClose} />);

    // Start the submit.
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    // While submitting, Escape should be suppressed.
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();

    // Resolve the submit.
    resolveConfirm!();
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
  });

  it("destructive=true gives the confirm button the destructive variant class", () => {
    render(<ConfirmDialog {...defaultProps} destructive={true} confirmLabel="Delete" />);
    const confirmBtn = screen.getByRole("button", { name: "Delete" });
    // The destructive variant uses bg-destructive/10 as the background class.
    expect(confirmBtn.className).toMatch(/bg-destructive/);
  });

  it("default (destructive=false) does not apply destructive background to confirm button", () => {
    render(<ConfirmDialog {...defaultProps} destructive={false} confirmLabel="Confirm" />);
    const confirmBtn = screen.getByRole("button", { name: "Confirm" });
    // Default variant uses bg-primary — not bg-destructive.
    expect(confirmBtn.className).toMatch(/bg-primary/);
    expect(confirmBtn.className).not.toMatch(/bg-destructive/);
  });
});
