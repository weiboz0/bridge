export { auth as middleware } from "@/lib/auth";

export const config = {
  matcher: ["/dashboard/:path*", "/api/classrooms/:path*", "/api/orgs/:path*", "/api/admin/:path*"],
};
