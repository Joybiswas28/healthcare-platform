"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useAuth } from "./auth-provider";
import { ThemeToggle } from "./theme-toggle";
import { Container } from "./ui";

// Header and footer sit on the fixed navy "chrome" colour in both themes,
// so their text uses white shades rather than the theme-aware greys.
const navLink = "rounded-md px-2 py-1 text-white/80 hover:bg-white/10 hover:text-white";

export function SiteHeader() {
  const { user, loading, logout } = useAuth();
  const router = useRouter();

  async function onLogout() {
    await logout();
    router.push("/");
  }

  return (
    <header className="border-b border-chrome-line bg-chrome">
      <Container className="flex h-16 items-center justify-between gap-4">
        <Link href="/" className="flex items-center gap-2 font-semibold text-white">
          <span className="grid h-8 w-8 place-items-center rounded-lg bg-teal-600 text-white" aria-hidden>
            +
          </span>
          {/* The full name is long; phones get the short form. */}
          <span className="hidden sm:inline">Digital Healthcare Platform</span>
          <span className="sm:hidden">DHP</span>
        </Link>

        <nav className="flex items-center gap-1 text-sm sm:gap-2">
          <ThemeToggle />
          <Link href="/doctors" className={navLink}>
            Doctors
          </Link>
          <Link href="/hospitals" className={navLink}>
            Hospitals
          </Link>
          <Link href="/pharmacies" className={`${navLink} hidden sm:inline`}>
            Pharmacies
          </Link>
          {loading ? null : user ? (
            <>
              {user.role === "admin" && (
                <Link href="/admin" className="rounded-md px-2 py-1 font-medium text-teal-400 hover:bg-white/10">
                  Admin
                </Link>
              )}
              {(user.role === "patient" || user.role === "doctor") && (
                <Link href="/consultations" className={navLink}>
                  Consult
                </Link>
              )}
              <Link href="/dashboard" className={navLink}>
                {user.name?.split(" ")[0] ?? "Account"}
              </Link>
              <button onClick={onLogout} className="rounded-md px-2 py-1 text-white/60 hover:bg-white/10 hover:text-white">
                Log out
              </button>
            </>
          ) : (
            <>
              <Link href="/login" className={navLink}>
                Log in
              </Link>
              <Link href="/register" className="rounded-lg bg-teal-700 px-3 py-1.5 font-medium text-white hover:bg-teal-600">
                Sign up
              </Link>
            </>
          )}
        </nav>
      </Container>
    </header>
  );
}
