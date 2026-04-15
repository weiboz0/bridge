import { cookies } from "next/headers";

const GO_API_URL = process.env.GO_API_URL || "http://localhost:8002";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public body?: unknown
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/**
 * Server-side API client. Reads the session cookie and forwards it
 * as a Bearer token to the Go backend.
 *
 * Usage (server component):
 *   const data = await api<MyType>("/api/courses");
 *   const data = await api<MyType>("/api/courses", { method: "POST", body: { title: "..." } });
 */
export async function api<T = unknown>(
  path: string,
  options: {
    method?: string;
    body?: unknown;
    headers?: Record<string, string>;
  } = {}
): Promise<T> {
  const cookieStore = await cookies();
  // Auth.js v5 cookie names (try HTTPS first, then HTTP, then v4 fallbacks)
  const sessionToken =
    cookieStore.get("__Secure-authjs.session-token")?.value ||
    cookieStore.get("authjs.session-token")?.value ||
    cookieStore.get("__Secure-next-auth.session-token")?.value ||
    cookieStore.get("next-auth.session-token")?.value;

  const impersonateCookie = cookieStore.get("bridge-impersonate")?.value;

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...options.headers,
  };

  if (sessionToken) {
    headers["Authorization"] = `Bearer ${sessionToken}`;
  }

  if (impersonateCookie) {
    headers["Cookie"] = `bridge-impersonate=${impersonateCookie}`;
  }

  const url = `${GO_API_URL}${path}`;
  const res = await fetch(url, {
    method: options.method || "GET",
    headers,
    body: options.body ? JSON.stringify(options.body) : undefined,
    cache: "no-store",
  });

  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new ApiError(res.status, `API ${res.status}: ${path}`, body);
  }

  if (res.status === 204) return undefined as T;

  return res.json() as Promise<T>;
}

/**
 * Client-side API path builder. Client components fetch relative paths
 * which Next.js proxies to Go, so no base URL needed.
 * If the proxy is removed, set NEXT_PUBLIC_GO_API_URL.
 */
export function clientApiUrl(path: string): string {
  const base = process.env.NEXT_PUBLIC_GO_API_URL || "";
  return `${base}${path}`;
}
