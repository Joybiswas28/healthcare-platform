import type { Metadata } from "next";
import { Geist } from "next/font/google";
import Link from "next/link";
import { AuthProvider } from "@/components/auth-provider";
import { SiteHeader } from "@/components/site-header";
import { Container } from "@/components/ui";
import { THEME_SCRIPT } from "@/lib/theme";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: { default: "Digital Healthcare Platform — Bangladesh", template: "%s · Digital Healthcare Platform" },
  description:
    "Find verified doctors, book a serial at their chamber, consult online, and order medicine from nearby pharmacies.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    // suppressHydrationWarning: the head script sets data-theme before React hydrates.
    <html lang="en" className={`${geistSans.variable} h-full antialiased`} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_SCRIPT }} />
      </head>
      <body className="flex min-h-full flex-col">
        <AuthProvider>
          <SiteHeader />
          <main className="flex-1">{children}</main>
          <footer className="border-t border-chrome-line bg-chrome py-6 text-sm text-white/70">
            <Container className="flex flex-wrap justify-between gap-2">
              <span>Digital Healthcare Platform · Bangladesh</span>
              <Link href="/verify" className="hover:text-white hover:underline">
                Verify a prescription
              </Link>
            </Container>
          </footer>
        </AuthProvider>
      </body>
    </html>
  );
}
