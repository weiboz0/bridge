import { describe, it, expect } from "vitest";
import * as schema from "@/lib/db/schema";

describe("Schema exports", () => {
  it("exports all table definitions", () => {
    expect(schema.users).toBeDefined();
    expect(schema.authProviders).toBeDefined();
    expect(schema.organizations).toBeDefined();
    expect(schema.orgMemberships).toBeDefined();
    expect(schema.classes).toBeDefined();
    expect(schema.classMemberships).toBeDefined();
    expect(schema.classSettings).toBeDefined();
    expect(schema.sessions).toBeDefined();
    expect(schema.sessionParticipants).toBeDefined();
    expect(schema.aiInteractions).toBeDefined();
    expect(schema.codeAnnotations).toBeDefined();
  });

  it("exports enum definitions", () => {
    expect(schema.authProviderEnum).toBeDefined();
    expect(schema.gradeLevelEnum).toBeDefined();
    expect(schema.editorModeEnum).toBeDefined();
    expect(schema.sessionStatusEnum).toBeDefined();
    expect(schema.participantStatusEnum).toBeDefined();
    expect(schema.annotationAuthorTypeEnum).toBeDefined();
    expect(schema.orgTypeEnum).toBeDefined();
    expect(schema.orgStatusEnum).toBeDefined();
    expect(schema.orgMemberRoleEnum).toBeDefined();
    expect(schema.orgMemberStatusEnum).toBeDefined();
  });

  it("users table has isPlatformAdmin and no role/schoolId", () => {
    const columns = Object.keys(schema.users);
    expect(columns).toContain("isPlatformAdmin");
    expect(columns).not.toContain("role");
    expect(columns).not.toContain("schoolId");
  });

  it("organizations table has expected columns", () => {
    const columns = Object.keys(schema.organizations);
    expect(columns).toContain("id");
    expect(columns).toContain("name");
    expect(columns).toContain("slug");
    expect(columns).toContain("type");
    expect(columns).toContain("status");
    expect(columns).toContain("domain");
  });

  it("orgMemberships table has expected columns", () => {
    const columns = Object.keys(schema.orgMemberships);
    expect(columns).toContain("orgId");
    expect(columns).toContain("userId");
    expect(columns).toContain("role");
    expect(columns).toContain("status");
    expect(columns).toContain("invitedBy");
  });

  it("problems has scope fields + topic_problems + problem_solutions", () => {
    expect(schema.problems.scope).toBeDefined();
    expect(schema.problems.starterCode).toBeDefined();
    expect(schema.topicProblems.problemId).toBeDefined();
    expect(schema.problemSolutions.isPublished).toBeDefined();
  });
});
