import { redirect } from "next/navigation";

export default function ChildrenPage() {
  redirect("/parent");
}
