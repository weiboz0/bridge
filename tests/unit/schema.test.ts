import { describe, it, expect } from "vitest";
import {
  users,
  authProviders,
  schools,
  classrooms,
  classroomMembers,
} from "@/lib/db/schema";

describe("database schema", () => {
  it("exports users table with expected columns", () => {
    const columns = Object.keys(users);
    expect(columns).toContain("id");
    expect(columns).toContain("name");
    expect(columns).toContain("email");
    expect(columns).toContain("role");
    expect(columns).toContain("schoolId");
    expect(columns).toContain("passwordHash");
    expect(columns).toContain("createdAt");
  });

  it("exports authProviders table with expected columns", () => {
    const columns = Object.keys(authProviders);
    expect(columns).toContain("id");
    expect(columns).toContain("userId");
    expect(columns).toContain("provider");
    expect(columns).toContain("providerUserId");
  });

  it("exports schools table with expected columns", () => {
    const columns = Object.keys(schools);
    expect(columns).toContain("id");
    expect(columns).toContain("name");
    expect(columns).toContain("settings");
  });

  it("exports classrooms table with expected columns", () => {
    const columns = Object.keys(classrooms);
    expect(columns).toContain("id");
    expect(columns).toContain("schoolId");
    expect(columns).toContain("teacherId");
    expect(columns).toContain("name");
    expect(columns).toContain("gradeLevel");
    expect(columns).toContain("editorMode");
    expect(columns).toContain("joinCode");
  });

  it("exports classroomMembers table with expected columns", () => {
    const columns = Object.keys(classroomMembers);
    expect(columns).toContain("classroomId");
    expect(columns).toContain("userId");
    expect(columns).toContain("joinedAt");
  });
});
