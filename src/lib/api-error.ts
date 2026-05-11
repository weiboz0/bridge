/**
 * Standalone `ApiError` so client components can import it without
 * pulling `next/headers` (server-only) via `api-client.ts`. The
 * server-side `api()` function in `api-client.ts` re-exports this.
 */
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
