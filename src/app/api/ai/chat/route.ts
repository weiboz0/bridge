import { NextRequest } from "next/server";
import { auth } from "@/lib/auth";
import { db } from "@/lib/db";
import {
  getAnthropicClient,
  getOpenAIClient,
  isAnthropicBackend,
  getModel,
} from "@/lib/ai/client";
import { getSystemPrompt } from "@/lib/ai/system-prompts";
import { filterResponse } from "@/lib/ai/guardrails";
import {
  getActiveInteraction,
  createInteraction,
  appendMessage,
} from "@/lib/ai/interactions";
import { getSession } from "@/lib/sessions";
import { getClassroom } from "@/lib/classrooms";

export async function POST(request: NextRequest) {
  const session = await auth();
  if (!session?.user?.id) {
    return new Response("Unauthorized", { status: 401 });
  }

  const body = await request.json();
  const { sessionId, message, code } = body;

  if (!sessionId || !message) {
    return new Response("Missing sessionId or message", { status: 400 });
  }

  const liveSession = await getSession(db, sessionId);
  if (!liveSession || liveSession.status !== "active") {
    return new Response("Session not found or ended", { status: 404 });
  }

  const classroom = await getClassroom(db, liveSession.classroomId);
  if (!classroom) {
    return new Response("Classroom not found", { status: 404 });
  }

  let interaction = await getActiveInteraction(db, session.user.id, sessionId);
  if (!interaction) {
    interaction = await createInteraction(db, {
      studentId: session.user.id,
      sessionId,
      enabledByTeacherId: liveSession.teacherId,
    });
  }

  await appendMessage(db, interaction.id, {
    role: "user",
    content: message,
    timestamp: new Date().toISOString(),
  });

  const history = ((interaction.messages as any[]) || []).map((m: any) => ({
    role: m.role as "user" | "assistant",
    content: m.content,
  }));
  history.push({ role: "user" as const, content: message });

  const systemPrompt =
    getSystemPrompt(classroom.gradeLevel as "K-5" | "6-8" | "9-12") +
    (code ? `\n\nThe student's current code:\n\`\`\`python\n${code}\n\`\`\`` : "");

  const model = getModel();

  const stream = new ReadableStream({
    async start(controller) {
      const encoder = new TextEncoder();
      let fullResponse = "";

      try {
        if (isAnthropicBackend()) {
          // --- Anthropic SDK ---
          const client = getAnthropicClient();
          const response = await client.messages.create({
            model,
            max_tokens: 500,
            system: systemPrompt,
            messages: history,
            stream: true,
          });

          for await (const event of response) {
            if (
              event.type === "content_block_delta" &&
              event.delta.type === "text_delta"
            ) {
              const text = event.delta.text;
              fullResponse += text;
              controller.enqueue(
                encoder.encode(`data: ${JSON.stringify({ text })}\n\n`)
              );
            }
          }
        } else {
          // --- OpenAI-compatible SDK ---
          const client = getOpenAIClient();
          const response = await client.chat.completions.create({
            model,
            max_tokens: 500,
            messages: [
              { role: "system", content: systemPrompt },
              ...history,
            ],
            stream: true,
          });

          for await (const chunk of response) {
            const text = chunk.choices[0]?.delta?.content;
            if (text) {
              fullResponse += text;
              controller.enqueue(
                encoder.encode(`data: ${JSON.stringify({ text })}\n\n`)
              );
            }
          }
        }

        // Apply guardrails to full response
        const filtered = filterResponse(fullResponse);
        if (filtered !== fullResponse) {
          controller.enqueue(
            encoder.encode(
              `data: ${JSON.stringify({ replace: filtered })}\n\n`
            )
          );
          fullResponse = filtered;
        }

        // Save assistant message
        await appendMessage(db, interaction!.id, {
          role: "assistant",
          content: fullResponse,
          timestamp: new Date().toISOString(),
        });

        controller.enqueue(encoder.encode("data: [DONE]\n\n"));
        controller.close();
      } catch (err: any) {
        controller.enqueue(
          encoder.encode(
            `data: ${JSON.stringify({ error: err.message })}\n\n`
          )
        );
        controller.close();
      }
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
