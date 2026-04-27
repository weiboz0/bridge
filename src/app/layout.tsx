import type { Metadata } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import Script from "next/script";
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
// Plan 040 phase 8: moved from inline `<script dangerouslySetInnerHTML>`
// (which triggered the dev-overlay "Encountered a script tag while
// rendering React component" error on every route) to next/script with
// `beforeInteractive` strategy, the supported pattern in App Router.
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
      <body className="min-h-full flex flex-col bg-background text-foreground font-sans">
        <Script
          id="bridge-theme-bootstrap"
          strategy="beforeInteractive"
        >
          {themeScript}
        </Script>
        <SessionProvider>
          <ImpersonateBanner />
          {children}
        </SessionProvider>
      </body>
    </html>
  );
}
