// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

// Mock Yjs
vi.mock("yjs", () => {
  const mockYText = {
    toString: vi.fn(() => 'print("hello")'),
    observe: vi.fn(),
    unobserve: vi.fn(),
  };
  class MockDoc {
    getText() {
      return mockYText;
    }
    destroy() {}
  }
  return {
    Doc: MockDoc,
    __mockYText: mockYText,
  };
});

// Mock HocuspocusProvider
vi.mock("@hocuspocus/provider", () => ({
  HocuspocusProvider: class MockHocuspocusProvider {
    awareness = {};
    destroy() {}
  },
}));

// Mock shiki
vi.mock("shiki", () => ({
  codeToHtml: vi.fn().mockResolvedValue(
    '<pre class="shiki"><code><span>print("hello")</span></code></pre>'
  ),
}));

// Mock AiToggleButton
vi.mock("@/components/ai/ai-toggle-button", () => ({
  AiToggleButton: () => <button data-testid="ai-toggle">AI</button>,
}));

import { StudentTile } from "@/components/session/student-tile";

describe("StudentTile", () => {
  const defaultProps = {
    sessionId: "session-1",
    studentId: "student-1",
    studentName: "Alice",
    status: "active",
    token: "test-token",
    onClick: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders student name", () => {
    render(<StudentTile {...defaultProps} />);
    expect(screen.getByText("Alice")).toBeInTheDocument();
  });

  it("renders status indicator with correct color", () => {
    const { container } = render(<StudentTile {...defaultProps} status="active" />);
    const dot = container.querySelector(".bg-green-500");
    expect(dot).toBeInTheDocument();
  });

  it("renders needs_help status with red indicator", () => {
    const { container } = render(<StudentTile {...defaultProps} status="needs_help" />);
    const dot = container.querySelector(".bg-red-500");
    expect(dot).toBeInTheDocument();
  });

  it("calls onClick when clicked", () => {
    render(<StudentTile {...defaultProps} />);
    fireEvent.click(screen.getByText("Alice").closest("[class*='cursor-pointer']")!);
    expect(defaultProps.onClick).toHaveBeenCalledTimes(1);
  });

  it("renders AI toggle button", () => {
    render(<StudentTile {...defaultProps} />);
    expect(screen.getByTestId("ai-toggle")).toBeInTheDocument();
  });

  it("renders code container", () => {
    const { container } = render(<StudentTile {...defaultProps} />);
    const codeContainer = container.querySelector(".h-24");
    expect(codeContainer).toBeInTheDocument();
  });
});
