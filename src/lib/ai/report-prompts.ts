export function getReportSystemPrompt(): string {
  return `You are a helpful education assistant generating a weekly progress report for a parent.

Write in a warm, encouraging, parent-friendly tone. Use simple language.
Structure the report with these sections:
1. **Summary** — 2-3 sentence overview
2. **Attendance** — sessions attended, any missed
3. **Progress** — what topics were covered, what skills improved
4. **Areas for Growth** — gentle suggestions, not criticism
5. **Teacher's Notes** — if annotations or feedback available

Keep it concise — under 300 words. Use the student's first name.`;
}

export function buildReportUserPrompt(data: {
  studentName: string;
  periodStart: string;
  periodEnd: string;
  sessionsAttended: number;
  sessionsTotal: number;
  topicsCovered: string[];
  assignmentGrades: Array<{ title: string; grade: number | null }>;
  aiInteractionCount: number;
  annotationCount: number;
}): string {
  const gradeList = data.assignmentGrades
    .map((a) => `- ${a.title}: ${a.grade !== null ? `${a.grade}/100` : "not graded"}`)
    .join("\n");

  return `Generate a progress report for ${data.studentName}.

Period: ${data.periodStart} to ${data.periodEnd}

Attendance: ${data.sessionsAttended} of ${data.sessionsTotal} sessions

Topics covered: ${data.topicsCovered.length > 0 ? data.topicsCovered.join(", ") : "None recorded"}

Assignments:
${gradeList || "No assignments yet"}

AI tutor interactions: ${data.aiInteractionCount} conversations
Teacher annotations: ${data.annotationCount} comments on code`;
}
