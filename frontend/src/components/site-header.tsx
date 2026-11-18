"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useAuth } from "./auth-provider";
import { Container } from "./ui";

export function SiteHeader() {
  const { user, loading, logout } = useAuth();
  const router = useRouter();

  async function onLogout() {
    await logout();
    router.push("/");
  }

  return (
    <header className="border-b border-slate-200 bg-white">
      <Container className="flex h-16 items-center justify-between gap-4">
        <Link href="/" className="flex items-center gap-2 font-semibold text-slate-900">
          <span className="grid h-8 w-8 place-items-center rounded-lg bg-teal-700 text-white" aria-hidden>
            +
          </span>
          <span>Shasthya Seba</span>
        </Link>

        <nav className="flex items-center gap-1 text-sm sm:gap-4">
          <Link href="/doctors" className="rounded-md px-2 py-1 text-slate-700 hover:text-teal-800">
            Doctors
          </Link>
          <Link href="/hospitals" className="rounded-md px-2 py-1 text-slate-700 hover:text-teal-800">
            Hospitals
          </Link>
          {loading ? null : user ? (
            <>
              {(user.role === "patient" || user.role === "doctor") && (
                <Link href="/consultations" className="rounded-md px-2 py-1 text-slate-700 hover:text-teal-800">
                  Consult
                </Link>
              )}
              <Link href="/dashboard" className="rounded-md px-2 py-1 text-slate-700 hover:text-teal-800">
                {user.name?.split(" ")[0] ?? "Account"}
              </Link>
              <button onClick={onLogout} className="rounded-md px-2 py-1 text-slate-500 hover:text-red-700">
                Log out
              </button>
            </>
          ) : (
            <>
              <Link href="/login" className="rounded-md px-2 py-1 text-slate-700 hover:text-teal-800">
                Log in
              </Link>
              <Link
                href="/register"
                className="rounded-lg bg-teal-700 px-3 py-1.5 font-medium text-white hover:bg-teal-800"
              >
                Sign up
              </Link>
            </>
          )}
        </nav>
      </Container>
    </header>
  );
}
