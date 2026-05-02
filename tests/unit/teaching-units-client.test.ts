import { afterEach, describe, expect, it, vi } from "vitest";
import { createUnit } from "@/lib/teaching-units";

// Plan 054 drift fix — `createUnit` was dropping `materialType` from
// the POST body. Without this assertion, the regression silently
// returns; the picker becomes decorative and every unit defaults to
// `notes`.

describe("createUnit (Plan 054 drift fix)", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("includes materialType in the POST body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "u-1", title: "Test" }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await createUnit({
      title: "Test Unit",
      scope: "personal",
      materialType: "worksheet",
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, init] = fetchMock.mock.calls[0];
    const body = JSON.parse(init.body);
    expect(body.materialType).toBe("worksheet");
  });

  it("defaults materialType to 'notes' when caller omits it", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ id: "u-2" }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      ),
    );

    await createUnit({ title: "T", scope: "personal" });
    const fetchMock = (globalThis.fetch as unknown) as ReturnType<typeof vi.fn>;
    const [, init] = fetchMock.mock.calls[0];
    const body = JSON.parse(init.body);
    expect(body.materialType).toBe("notes");
  });

  it("accepts every valid materialType variant", async () => {
    const variants = ["notes", "slides", "worksheet", "reference"] as const;
    for (const m of variants) {
      const fetchMock = vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ id: "u" }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
      vi.stubGlobal("fetch", fetchMock);

      await createUnit({ title: "T", scope: "personal", materialType: m });
      const [, init] = fetchMock.mock.calls[0];
      expect(JSON.parse(init.body).materialType).toBe(m);
    }
  });
});
