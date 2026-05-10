// Plan 080: server-side redirect placeholder. Don't re-add page content
// here without revisiting the parent-portal nav decision (browser review
// 011-2026-05-09 §P1 #4). Per-child reports live at
// /parent/children/[id]/reports; the canonical entry point is the
// dashboard at /parent.
import { redirect } from "next/navigation";

export default function ParentReportsPage() {
  redirect("/parent");
}
