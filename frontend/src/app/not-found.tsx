import type { Metadata } from "next";
import Link from "next/link";
import { NotFoundCard } from "@/components/load-failure";
import { Container } from "@/components/ui";

export const metadata: Metadata = { title: "Page not found" };

const PLACES = [
  { href: "/doctors", label: "Find a doctor" },
  { href: "/hospitals", label: "Hospitals" },
  { href: "/pharmacies", label: "Pharmacies" },
  { href: "/dashboard", label: "My dashboard" },
];

// Shown for any address that doesn't match a page.
export default function NotFound() {
  return (
    <Container className="py-16">
      <NotFoundCard title="Page not found">
        There is no page at this address. It may have moved, or the link has a typo.
      </NotFoundCard>
      <nav className="mx-auto mt-6 flex max-w-lg flex-wrap justify-center gap-x-5 gap-y-2 text-sm" aria-label="Popular pages">
        {PLACES.map((p) => (
          <Link key={p.href} href={p.href} className="text-teal-800 hover:underline">
            {p.label}
          </Link>
        ))}
      </nav>
    </Container>
  );
}
