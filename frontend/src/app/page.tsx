import Link from "next/link";
import { DoctorSearchForm } from "@/components/doctor-search-form";
import { Container } from "@/components/ui";

const SERVICES = [
  {
    title: "Book a doctor's serial",
    body: "Find a verified doctor, pick a chamber session and get your serial number and e-ticket instantly.",
    href: "/doctors",
    live: true,
  },
  {
    title: "Consult online",
    body: "Chat with a doctor from home using consultation tokens, or post a concern and receive offers.",
    live: false,
  },
  {
    title: "Hospitals",
    body: "Browse hospitals, their doctors and chambers, and book through the hospital directly.",
    live: false,
  },
  {
    title: "Medicine delivery",
    body: "Send your prescription to nearby pharmacies, compare quotes and get medicine delivered.",
    live: false,
  },
];

export default function Home() {
  return (
    <>
      <section className="bg-gradient-to-b from-teal-50 to-transparent">
        <Container className="py-14 sm:py-20">
          <h1 className="max-w-2xl text-3xl font-semibold tracking-tight text-slate-900 sm:text-4xl">
            Healthcare across Bangladesh, a few taps away
          </h1>
          <p className="mt-4 max-w-xl text-slate-600">
            Search verified doctors by name, specialty or district and book your serial without waiting in line.
          </p>
          <div className="mt-8 max-w-3xl">
            <DoctorSearchForm />
          </div>
        </Container>
      </section>

      <Container className="grid gap-4 py-12 sm:grid-cols-2">
        {SERVICES.map((s) => {
          const body = (
            <>
              <div className="flex items-center justify-between gap-2">
                <h2 className="font-semibold text-slate-900">{s.title}</h2>
                {!s.live && (
                  <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-500">Coming soon</span>
                )}
              </div>
              <p className="mt-2 text-sm text-slate-600">{s.body}</p>
            </>
          );
          return s.href ? (
            <Link
              key={s.title}
              href={s.href}
              className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm transition hover:border-teal-600"
            >
              {body}
            </Link>
          ) : (
            <div key={s.title} className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
              {body}
            </div>
          );
        })}
      </Container>
    </>
  );
}
