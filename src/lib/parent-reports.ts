import { eq, and, gte, lte, desc } from "drizzle-orm";
import { parentReports, submissions, aiInteractions, codeAnnotations, liveSessions, sessionParticipants, sessionTopics, topics } from "@/lib/db/schema";
import type { Database } from "@/lib/db";
import { getReportSystemPrompt, buildReportUserPrompt } from "@/lib/ai/report-prompts";
import { isAnthropicBackend, getAnthropicClient, getOpenAIClient, getModel } from "@/lib/ai/client";

interface GenerateReportInput {
  studentId: string;
  studentName: string;
  generatedBy: string;
  periodStart: Date;
  periodEnd: Date;
}

export async function generateReport(
  db: Database,
  input: GenerateReportInput
) {
  const { studentId, studentName, generatedBy, periodStart, periodEnd } = input;

  // Gather data for the period
  const participations = await db
    .select({ sessionId: sessionParticipants.sessionId })
    .from(sessionParticipants)
    .innerJoin(liveSessions, eq(sessionParticipants.sessionId, liveSessions.id))
    .where(
      and(
        eq(sessionParticipants.studentId, studentId),
        gte(liveSessions.startedAt, periodStart),
        lte(liveSessions.startedAt, periodEnd)
      )
    );

  const totalSessions = await db
    .select()
    .from(liveSessions)
    .where(
      and(
        gte(liveSessions.startedAt, periodStart),
        lte(liveSessions.startedAt, periodEnd)
      )
    );

  // Get topics covered
  const topicsCovered: string[] = [];
  for (const p of participations) {
    const st = await db
      .select({ title: topics.title })
      .from(sessionTopics)
      .innerJoin(topics, eq(sessionTopics.topicId, topics.id))
      .where(eq(sessionTopics.sessionId, p.sessionId));
    topicsCovered.push(...st.map((t) => t.title));
  }

  // Get grades
  const studentSubmissions = await db
    .select()
    .from(submissions)
    .where(eq(submissions.studentId, studentId));

  // Get AI interaction count
  const aiCount = await db
    .select()
    .from(aiInteractions)
    .where(eq(aiInteractions.studentId, studentId));

  // Get annotation count
  const annotationCount = 0; // Annotations are by documentId, not studentId — approximate

  // Build prompt
  const userPrompt = buildReportUserPrompt({
    studentName,
    periodStart: periodStart.toLocaleDateString(),
    periodEnd: periodEnd.toLocaleDateString(),
    sessionsAttended: participations.length,
    sessionsTotal: totalSessions.length,
    topicsCovered: [...new Set(topicsCovered)],
    assignmentGrades: studentSubmissions.map((s) => ({
      title: `Assignment`,
      grade: s.grade,
    })),
    aiInteractionCount: aiCount.length,
    annotationCount,
  });

  // Generate with LLM
  let content: string;
  const systemPrompt = getReportSystemPrompt();
  const model = getModel();

  if (isAnthropicBackend()) {
    const client = getAnthropicClient();
    const response = await client.messages.create({
      model,
      max_tokens: 500,
      system: systemPrompt,
      messages: [{ role: "user", content: userPrompt }],
    });
    content = response.content[0].type === "text" ? response.content[0].text : "";
  } else {
    const client = getOpenAIClient();
    const response = await client.chat.completions.create({
      model,
      max_tokens: 500,
      messages: [
        { role: "system", content: systemPrompt },
        { role: "user", content: userPrompt },
      ],
    });
    content = response.choices[0]?.message?.content || "";
  }

  // Save report
  const [report] = await db
    .insert(parentReports)
    .values({
      studentId,
      generatedBy,
      periodStart,
      periodEnd,
      content,
      summary: {
        sessionsAttended: participations.length,
        sessionsTotal: totalSessions.length,
        topicsCovered: [...new Set(topicsCovered)],
        aiInteractions: aiCount.length,
      },
    })
    .returning();

  return report;
}

export async function listReports(db: Database, studentId: string) {
  return db
    .select()
    .from(parentReports)
    .where(eq(parentReports.studentId, studentId))
    .orderBy(desc(parentReports.createdAt));
}

export async function getReport(db: Database, reportId: string) {
  const [report] = await db
    .select()
    .from(parentReports)
    .where(eq(parentReports.id, reportId));
  return report || null;
}
