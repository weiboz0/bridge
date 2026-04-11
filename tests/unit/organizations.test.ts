import { describe, it, expect, beforeEach } from "vitest";
import { testDb, createTestOrg } from "../helpers";
import {
  createOrganization,
  getOrganization,
  getOrganizationBySlug,
  listOrganizations,
  updateOrgStatus,
} from "@/lib/organizations";

describe("organization operations", () => {
  it("creates an organization", async () => {
    const org = await createOrganization(testDb, {
      name: "Lincoln High",
      slug: "lincoln-high",
      type: "school",
      contactEmail: "admin@lincoln.edu",
      contactName: "Admin",
    });
    expect(org.id).toBeDefined();
    expect(org.name).toBe("Lincoln High");
    expect(org.status).toBe("pending");
  });

  it("gets organization by ID", async () => {
    const org = await createTestOrg({ name: "Test School" });
    const found = await getOrganization(testDb, org.id);
    expect(found).not.toBeNull();
    expect(found!.name).toBe("Test School");
  });

  it("gets organization by slug", async () => {
    const org = await createTestOrg({ slug: "unique-slug" });
    const found = await getOrganizationBySlug(testDb, "unique-slug");
    expect(found).not.toBeNull();
    expect(found!.id).toBe(org.id);
  });

  it("returns null for non-existent org", async () => {
    const found = await getOrganization(testDb, "00000000-0000-0000-0000-000000000000");
    expect(found).toBeNull();
  });

  it("lists organizations by status", async () => {
    await createTestOrg({ status: "pending" });
    await createTestOrg({ status: "active" });
    await createTestOrg({ status: "active" });

    const pending = await listOrganizations(testDb, "pending");
    expect(pending).toHaveLength(1);

    const active = await listOrganizations(testDb, "active");
    expect(active).toHaveLength(2);

    const all = await listOrganizations(testDb);
    expect(all).toHaveLength(3);
  });

  it("updates org status to active and sets verifiedAt", async () => {
    const org = await createTestOrg({ status: "pending" });
    const updated = await updateOrgStatus(testDb, org.id, "active");
    expect(updated!.status).toBe("active");
    expect(updated!.verifiedAt).not.toBeNull();
  });

  it("suspends an org", async () => {
    const org = await createTestOrg({ status: "active" });
    const updated = await updateOrgStatus(testDb, org.id, "suspended");
    expect(updated!.status).toBe("suspended");
  });
});
