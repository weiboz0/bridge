// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { TeachersList } from "@/components/org/teachers-list";
import { StudentsList } from "@/components/org/students-list";
import { CoursesList, type OrgCourseRow } from "@/components/org/courses-list";
import { ClassesList, type OrgClassRow } from "@/components/org/classes-list";
import { OrgSettingsCard, type OrgSettingsData } from "@/components/org/org-settings-card";
import type { OrgMemberRow } from "@/components/org/teachers-list";

const teacherRows: OrgMemberRow[] = [
  { userId: "u1", name: "Eve Teacher", email: "eve@demo.edu", role: "teacher", joinedAt: "2026-01-15T00:00:00Z" },
];
const studentRows: OrgMemberRow[] = [
  { userId: "u2", name: "Alice Student", email: "alice@demo.edu", role: "student", joinedAt: "2026-02-10T00:00:00Z" },
];
const courseRows: OrgCourseRow[] = [
  { id: "c1", title: "Intro Python", gradeLevel: "K-5", language: "python", createdAt: "2026-03-01T00:00:00Z" },
];
const classRows: OrgClassRow[] = [
  {
    id: "cls1",
    title: "Period 3",
    term: "fall",
    status: "active",
    courseId: "c1",
    courseTitle: "Intro Python",
    instructorCount: 1,
    studentCount: 12,
    createdAt: "2026-03-15T00:00:00Z",
  },
];
const settingsData: OrgSettingsData = {
  id: "org1",
  name: "Bridge Demo School",
  type: "school",
  status: "active",
  contactEmail: "admin@demo.edu",
  contactName: "Frank OrgAdmin",
  domain: "demo.edu",
  verifiedAt: "2026-01-01T00:00:00Z",
};

describe("TeachersList", () => {
  it("renders rows when populated", () => {
    render(<TeachersList data={teacherRows} error={null} />);
    expect(screen.getByText("Eve Teacher")).toBeInTheDocument();
    expect(screen.getByText("eve@demo.edu")).toBeInTheDocument();
  });

  it("renders empty-state copy on empty list", () => {
    render(<TeachersList data={[]} error={null} />);
    expect(screen.getByText(/No teachers yet/i)).toBeInTheDocument();
  });

  it("renders error card on 403", () => {
    render(<TeachersList data={null} error={{ status: 403, message: "Forbidden" }} />);
    expect(screen.getByText(/HTTP 403/i)).toBeInTheDocument();
    expect(screen.getByText(/api\/auth\/debug/i)).toBeInTheDocument();
  });

  it("renders error card without status hint on 500", () => {
    render(<TeachersList data={null} error={{ status: 500, message: "boom" }} />);
    expect(screen.getByText(/HTTP 500/i)).toBeInTheDocument();
    expect(screen.queryByText(/api\/auth\/debug/i)).not.toBeInTheDocument();
  });
});

describe("StudentsList", () => {
  it("renders rows", () => {
    render(<StudentsList data={studentRows} error={null} />);
    expect(screen.getByText("Alice Student")).toBeInTheDocument();
  });

  it("renders empty-state copy", () => {
    render(<StudentsList data={[]} error={null} />);
    expect(screen.getByText(/No students yet/i)).toBeInTheDocument();
  });
});

describe("CoursesList", () => {
  it("renders title + grade + language", () => {
    render(<CoursesList data={courseRows} error={null} />);
    expect(screen.getByText("Intro Python")).toBeInTheDocument();
    expect(screen.getByText("K-5")).toBeInTheDocument();
    expect(screen.getByText("python")).toBeInTheDocument();
  });

  it("renders empty-state copy", () => {
    render(<CoursesList data={[]} error={null} />);
    expect(screen.getByText(/No courses yet/i)).toBeInTheDocument();
  });
});

describe("ClassesList", () => {
  it("renders title, course, term, instructor + student counts", () => {
    render(<ClassesList data={classRows} error={null} />);
    expect(screen.getByText("Period 3")).toBeInTheDocument();
    expect(screen.getByText("Intro Python")).toBeInTheDocument();
    expect(screen.getByText("fall")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
  });

  it("renders empty-state copy", () => {
    render(<ClassesList data={[]} error={null} />);
    expect(screen.getByText(/No classes yet/i)).toBeInTheDocument();
  });

  it("falls back to 'unlinked' when courseTitle is empty", () => {
    const orphan: OrgClassRow = { ...classRows[0], courseTitle: "" };
    render(<ClassesList data={[orphan]} error={null} />);
    expect(screen.getByText(/unlinked/i)).toBeInTheDocument();
  });
});

describe("OrgSettingsCard", () => {
  it("renders the org name + metadata fields", () => {
    render(<OrgSettingsCard org={settingsData} error={null} />);
    expect(screen.getByText("Bridge Demo School")).toBeInTheDocument();
    expect(screen.getByText("school")).toBeInTheDocument();
    expect(screen.getByText("admin@demo.edu")).toBeInTheDocument();
    expect(screen.getByText("Frank OrgAdmin")).toBeInTheDocument();
    expect(screen.getByText("demo.edu")).toBeInTheDocument();
  });

  it("renders 'no organization' copy when org is null", () => {
    render(<OrgSettingsCard org={null} error={null} />);
    expect(screen.getByText(/No organization is associated/i)).toBeInTheDocument();
  });

  it("renders error card on failure", () => {
    render(<OrgSettingsCard org={null} error={{ status: 500, message: "boom" }} />);
    expect(screen.getByText(/HTTP 500/i)).toBeInTheDocument();
  });
});
