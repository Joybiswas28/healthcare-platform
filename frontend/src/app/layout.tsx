import type { Metadata } from "next";
import { Geist, Noto_Sans_Bengali } from "next/font/google";
import Link from "next/link";
import { AuthProvider } from "@/components/auth-provider";
import { SiteHeader } from "@/components/site-header";
import { T } from "@/components/t";
import { Container } from "@/components/ui";
import { LangRemount } from "@/components/use-lang";
import { LANG_SCRIPT } from "@/lib/i18n";
import { THEME_SCRIPT } from "@/lib/theme";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

// Only fetched when Bangla text is on screen (unicode-range), so English readers
// don't download it.
const bengali = Noto_Sans_Bengali({
  variable: "--font-bengali",
  subsets: ["bengali"],
  preload: false,
});

export const metadata: Metadata = {
  title: { default: "Digital Healthcare Platform — Bangladesh", template: "%s · Digital Healthcare Platform" },
  description:
    "Find verified doctors, book a serial at their chamber, consult online, and order medicine from nearby pharmacies.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    // suppressHydrationWarning: the head scripts set data-theme and lang before React hydrates.
    <html lang="en" className={`${geistSans.variable} ${bengali.variable} h-full antialiased`} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_SCRIPT + ";" + LANG_SCRIPT }} />
      </head>
      <body className="flex min-h-full flex-col">
        <AuthProvider>
          <SiteHeader />
          <main className="flex-1">
            <LangRemount>{children}</LangRemount>
          </main>
          <footer className="border-t border-chrome-line bg-chrome py-6 text-sm text-white/70">
            <Container className="flex flex-wrap justify-between gap-2">
              <span>
                <T>Digital Healthcare Platform · Bangladesh</T>
              </span>
              <Link href="/verify" className="hover:text-white hover:underline">
                <T>Verify a prescription</T>
              </Link>
            </Container>
          </footer>
        </AuthProvider>
      </body>
    </html>
  );
}
