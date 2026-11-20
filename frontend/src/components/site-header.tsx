"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import type { User } from "@/lib/api/client";
import { useAuth } from "./auth-provider";
import { NotificationBell } from "./notification-bell";
import { T } from "./t";
import { ThemeToggle } from "./theme-toggle";
import { LangToggle, useT } from "./use-lang";
import { Container } from "./ui";

// Header and footer sit on the fixed navy "chrome" colour in both themes,
// so their text uses white shades rather than the theme-aware greys.
const navLink = "rounded-md px-2 py-1 text-white/80 hover:bg-white/10 hover:text-white";
const menuLink = "block rounded-lg px-3 py-2.5 text-base text-white/90 hover:bg-white/10";

type NavItem = { href: string; label: string; accent?: boolean };

function navItems(user: User | null): NavItem[] {
  const items: NavItem[] = [
    { href: "/doctors", label: "Doctors" },
    { href: "/hospitals", label: "Hospitals" },
    { href: "/pharmacies", label: "Pharmacies" },
  ];
  if (user?.role === "admin") items.unshift({ href: "/admin", label: "Admin", accent: true });
  if (user?.role === "patient" || user?.role === "doctor") items.push({ href: "/consultations", label: "Consult" });
  return items;
}

export function SiteHeader() {
  const { user, loading, logout } = useAuth();
  const router = useRouter();
  const t = useT();
  const [menuOpen, setMenuOpen] = useState(false);
  // Tapping any link in the phone menu closes it. (Reading the current path to do
  // this would make the root layout dynamic under Cache Components.)
  const close = () => setMenuOpen(false);

  useEffect(() => {
    if (!menuOpen) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setMenuOpen(false);
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [menuOpen]);

  async function onLogout() {
    setMenuOpen(false);
    await logout();
    router.push("/");
  }

  const items = navItems(user);
  const firstName = user?.name?.split(" ")[0] ?? "Account";

  return (
    <header className="relative border-b border-chrome-line bg-chrome">
      <Container className="flex h-16 items-center justify-between gap-3">
        <Link href="/" className="flex min-w-0 items-center gap-2 font-semibold text-white">
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-teal-600 text-white" aria-hidden>
            +
          </span>
          {/* The full name is long; small screens get the short form. */}
          <span className="hidden truncate lg:inline">
            <T>Digital Healthcare Platform</T>
          </span>
          <span className="lg:hidden">DHP</span>
        </Link>

        <div className="flex items-center gap-1 text-sm">
          {/* Wide screens: the section links in the row. */}
          <nav className="hidden items-center gap-1 md:flex" aria-label="Main">
            {items.map((i) => (
              <Link
                key={i.href}
                href={i.href}
                className={i.accent ? "rounded-md px-2 py-1 font-medium text-teal-400 hover:bg-white/10" : navLink}
              >
                <T>{i.label}</T>
              </Link>
            ))}
          </nav>

          {/* Always visible, rendered once for both layouts. */}
          <LangToggle />
          <ThemeToggle />
          {user && <NotificationBell role={user.role} />}

          {/* Wide screens: account links. */}
          <div className="hidden items-center gap-1 md:flex">
            {loading ? null : user ? (
              <>
                <Link href="/dashboard" className={navLink}>
                  {firstName}
                </Link>
                <button onClick={onLogout} className="rounded-md px-2 py-1 text-white/60 hover:bg-white/10 hover:text-white">
                  <T>Log out</T>
                </button>
              </>
            ) : (
              <>
                <Link href="/login" className={navLink}>
                  <T>Log in</T>
                </Link>
                <Link href="/register" className="rounded-lg bg-teal-700 px-3 py-1.5 font-medium text-white hover:bg-teal-600">
                  <T>Sign up</T>
                </Link>
              </>
            )}
          </div>

          {/* Phones: the rest goes in the menu. */}
          <button
            type="button"
            onClick={() => setMenuOpen((o) => !o)}
            className="grid h-9 w-9 place-items-center rounded-lg text-white hover:bg-white/10 md:hidden"
            aria-label={menuOpen ? t("Close menu") : t("Open menu")}
            aria-expanded={menuOpen}
            aria-controls="mobile-menu"
          >
            <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
              {menuOpen ? <path d="M6 6l12 12M18 6L6 18" /> : <path d="M4 7h16M4 12h16M4 17h16" />}
            </svg>
          </button>
        </div>
      </Container>

      {menuOpen && (
        <nav
          id="mobile-menu"
          aria-label="Main"
          className="absolute inset-x-0 top-full z-40 border-b border-chrome-line bg-chrome shadow-xl md:hidden"
        >
          <Container className="space-y-1 py-3">
            {user && (
              <p className="px-3 pb-1 text-xs uppercase tracking-wider text-white/50"><T vars={{ name: user.name }}>{"Signed in as {name}"}</T></p>
            )}
            {items.map((i) => (
              <Link
                key={i.href}
                href={i.href}
                onClick={close}
                className={`${menuLink} ${i.accent ? "font-medium text-teal-400" : ""}`}
              >
                <T>{i.label}</T>
              </Link>
            ))}
            <div className="my-2 border-t border-white/10" />
            {loading ? null : user ? (
              <>
                <Link href="/dashboard" onClick={close} className={menuLink}>
                  <T>My dashboard</T>
                </Link>
                <Link href="/notifications" onClick={close} className={menuLink}>
                  <T>Notifications</T>
                </Link>
                <Link href="/settings" onClick={close} className={menuLink}>
                  <T>Account settings</T>
                </Link>
                <button onClick={onLogout} className={`${menuLink} w-full text-left text-white/70`}>
                  <T>Log out</T>
                </button>
              </>
            ) : (
              <div className="flex gap-2 px-3 pt-1">
                <Link
                  href="/login"
                  onClick={close}
                  className="flex-1 rounded-lg border border-white/20 px-3 py-2.5 text-center text-white hover:bg-white/10"
                >
                  <T>Log in</T>
                </Link>
                <Link
                  href="/register"
                  onClick={close}
                  className="flex-1 rounded-lg bg-teal-700 px-3 py-2.5 text-center font-medium text-white hover:bg-teal-600"
                >
                  <T>Sign up</T>
                </Link>
              </div>
            )}
          </Container>
        </nav>
      )}
    </header>
  );
}
