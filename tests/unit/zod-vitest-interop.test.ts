import { describe, expect, it } from "vitest";
import { POST } from "@/app/api/auth/signup-intent/route";
import { z } from "zod";
import { ZodError } from "zod/v4";

describe("Zod Vitest interop", () => {
  it("evaluates an app Zod consumer and shares validation errors with zod/v4", async () => {
    const response = await POST(new Request("http://localhost/api/auth/signup-intent", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ role: "not-a-role" }),
    }) as never);
    const schema = z.object({ name: z.string() });

    let thrown: unknown;
    try {
      schema.parse({ name: 42 });
    } catch (error) {
      thrown = error;
    }

    expect(response.status).toBe(400);
    expect(z.object).toBeTypeOf("function");
    expect(thrown).toBeInstanceOf(ZodError);
  });
});
