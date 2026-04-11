import { PortalShell } from "@/components/portal/portal-shell";

export default function Layout({ children }: { children: React.ReactNode }) {
  return <PortalShell portalRole="student">{children}</PortalShell>;
}
