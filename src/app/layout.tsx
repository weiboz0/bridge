import type { Metadata } from "next";
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

// Theme bootstrap — runs before React hydration to prevent FOUC.
//
// Plan 048 phase 2: rendered as a literal <script> inside <head> via
// dangerouslySetInnerHTML. Plan 040's next/script + beforeInteractive
// approach inside <body> still triggered the dev-overlay warning
// "Encountered a script tag while rendering React component" in
// Next 16. The supported App Router pattern for sync inline scripts
// that need to run before hydration is to render them in <head>.
//
// e2e/theme-bootstrap.spec.ts is the regression gate.
const themeScript = `(function(){var t=localStorage.getItem('bridge-theme')||'light';if(t==='dark'){document.documentElement.classList.add('dark');}else{document.documentElement.classList.remove('dark');}})();`;

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${inter.variable} ${jetbrainsMono.variable} h-full antialiased`}
      suppressHydrationWarning
    >
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className="min-h-full flex flex-col bg-background text-foreground font-sans">
        <SessionProvider>
          <ImpersonateBanner />
          {children}
        </SessionProvider>
      </body>
    </html>
  );
}
