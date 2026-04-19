export { auth as middleware } from "@/lib/auth";

export const config = {
  matcher: ["/api/orgs/:path*", "/api/admin/:path*"],
};
