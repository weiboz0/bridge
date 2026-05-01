import type { Metadata } from "next";
import { cookies } from "next/headers";
import { Inter, JetBrains_Mono } from "next/font/google";
import "./globals.css";
import { SessionProvider } from "@/components/session-provider";
import { ImpersonateBanner } from "@/components/admin/impersonate-banner";

const inter = Inter({
  variable: "--font-sans",
  subsets: ["latin"],
});

const jetbrainsMono = JetBrains_Mono({
  variable: "--font-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Bridge - Learn to Code",
  description: "A live-first K-12 coding education platform",
};

// Theme is read server-side from the `bridge-theme` cookie and
// applied as a class on <html>. Replaces the prior inline-script
// FOUC-prevention dance (plan 040 / 048) which kept tripping
// React 19 + Next 16's "Encountered a script tag while rendering
// React component" dev warning. The cookie is written by
// src/lib/hooks/use-theme.ts on every theme change, alongside
// localStorage (kept for backwards compat with prior deployments).
// e2e/theme-bootstrap.spec.ts is the regression gate.
export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const cookieStore = await cookies();
  const isDark = cookieStore.get("bridge-theme")?.value === "dark";
  return (
    <html
      lang="en"
      className={`${inter.variable} ${jetbrainsMono.variable} h-full antialiased${isDark ? " dark" : ""}`}
      suppressHydrationWarning
    >
      <body className="min-h-full flex flex-col bg-background text-foreground font-sans">
        <SessionProvider>
          <ImpersonateBanner />
          {children}
        </SessionProvider>
      </body>
    </html>
  );
}
