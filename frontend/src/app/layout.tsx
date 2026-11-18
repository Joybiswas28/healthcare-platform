import type { Metadata } from "next";
import { Geist } from "next/font/google";
import { AuthProvider } from "@/components/auth-provider";
import { SiteHeader } from "@/components/site-header";
import { Container } from "@/components/ui";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: { default: "Shasthya Seba — Digital healthcare for Bangladesh", template: "%s · Shasthya Seba" },
  description:
    "Find verified doctors, book a serial at their chamber, consult online, and order medicine from nearby pharmacies.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${geistSans.variable} h-full antialiased`}>
      <body className="flex min-h-full flex-col">
        <AuthProvider>
          <SiteHeader />
          <main className="flex-1">{children}</main>
          <footer className="border-t border-slate-200 bg-white py-6 text-sm text-slate-500">
            <Container>Shasthya Seba · Digital healthcare platform for Bangladesh</Container>
          </footer>
        </AuthProvider>
      </body>
    </html>
  );
}
