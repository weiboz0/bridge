import { NextRequest } from "next/server";
import { auth } from "@/lib/auth";
import { sessionEventBus } from "@/lib/sse";

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const session = await auth();
  if (!session?.user?.id) {
    return new Response("Unauthorized", { status: 401 });
  }

  const { id } = await params;

  const stream = new ReadableStream({
    start(controller) {
      const encoder = new TextEncoder();

      const unsubscribe = sessionEventBus.subscribe(id, (event, data) => {
        const message = `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;
        controller.enqueue(encoder.encode(message));
      });

      // Send initial ping
      controller.enqueue(encoder.encode(`event: connected\ndata: {}\n\n`));

      // Cleanup on close
      _request.signal.addEventListener("abort", () => {
        unsubscribe();
        controller.close();
      });
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    },
  });
}
