import { describe, expect, it } from "vitest";
import { ZodError, z } from "zod";

describe("Zod Vitest interop", () => {
  it("preserves named exports and validation error identity", () => {
    const schema = z.object({ name: z.string() });

    let thrown: unknown;
    try {
      schema.parse({ name: 42 });
    } catch (error) {
      thrown = error;
    }

    expect(z.object).toBeTypeOf("function");
    expect(thrown).toBeInstanceOf(ZodError);
  });
});
