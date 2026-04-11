/**
 * Integration test helpers for calling Next.js API route handlers directly.
 *
 * Mocks the auth() function so routes see a fake session,
 * then calls the handler with a real NextRequest.
 */
import { vi } from "vitest";
import { NextRequest } from "next/server";

interface MockUser {
  id: string;
  name: string;
  email: string;
  isPlatformAdmin?: boolean;
}

let mockUser: MockUser | null = null;

/**
 * Set the mock authenticated user for subsequent route handler calls.
 * Pass null to simulate unauthenticated requests.
 */
export function setMockUser(user: MockUser | null) {
  mockUser = user;
}

// Mock the auth module — vi.mock is hoisted by Vitest
vi.mock("@/lib/auth", () => ({
  auth: vi.fn(async () => {
    if (!mockUser) return null;
    return {
      user: {
        id: mockUser.id,
        name: mockUser.name,
        email: mockUser.email,
        isPlatformAdmin: mockUser.isPlatformAdmin || false,
      },
    };
  }),
  handlers: { GET: vi.fn(), POST: vi.fn() },
  signIn: vi.fn(),
  signOut: vi.fn(),
}));

// Mock the db module to use the test database
vi.mock("@/lib/db", async () => {
  const { drizzle } = await import("drizzle-orm/postgres-js");
  const postgres = (await import("postgres")).default;
  const schema = await import("@/lib/db/schema");

  const testClient = postgres(
    process.env.DATABASE_URL || "postgresql://work@127.0.0.1:5432/bridge_test"
  );
  const db = drizzle(testClient, { schema });

  return { db };
});

/**
 * Create a NextRequest for testing API route handlers.
 */
export function createRequest(
  url: string,
  options: {
    method?: string;
    body?: unknown;
    searchParams?: Record<string, string>;
  } = {}
): NextRequest {
  const { method = "GET", body, searchParams } = options;

  const fullUrl = new URL(url, "http://localhost:3000");
  if (searchParams) {
    for (const [key, value] of Object.entries(searchParams)) {
      fullUrl.searchParams.set(key, value);
    }
  }

  const init: any = { method };
  if (body) {
    init.body = JSON.stringify(body);
    init.headers = { "Content-Type": "application/json" };
  }

  return new NextRequest(fullUrl, init);
}

/**
 * Parse JSON response body from a route handler Response.
 */
export async function parseResponse<T = unknown>(
  response: Response
): Promise<{ status: number; body: T }> {
  const status = response.status;
  const text = await response.text();
  try {
    const body = JSON.parse(text) as T;
    return { status, body };
  } catch {
    return { status, body: text as unknown as T };
  }
}
