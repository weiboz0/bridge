import { PortalShell } from "@/components/portal/portal-shell";

// Role-neutral layout: any authenticated portal user (≥1 role) can access
// /library. See plan 089 Decision #13 and PortalShell's null-role gate.
export default function LibraryLayout({ children }: { children: React.ReactNode }) {
  return <PortalShell portalRole={null}>{children}</PortalShell>;
}
