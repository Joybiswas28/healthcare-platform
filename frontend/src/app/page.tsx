import Image from "next/image";
import Link from "next/link";
import { DoctorSearchForm } from "@/components/doctor-search-form";
import heroImage from "@/assets/hero-doctor.jpg";
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
    href: "/consultations",
    live: true,
  },
  {
    title: "Hospitals",
    body: "Browse hospitals, their doctors and chambers, and book through the hospital directly.",
    href: "/hospitals",
    live: true,
  },
  {
    title: "Medicine delivery",
    body: "Send your prescription to nearby pharmacies, compare quotes and get medicine delivered.",
    href: "/medicines/new",
    live: true,
  },
];

export default function Home() {
  return (
    <>
      <section className="bg-gradient-to-b from-teal-50 to-transparent">
        <Container className="grid items-center gap-8 py-12 sm:py-16 lg:grid-cols-2 lg:gap-12">
          <div>
            <p className="text-sm font-semibold uppercase tracking-wider text-teal-800">Digital Healthcare Platform</p>
            <h1 className="mt-2 max-w-2xl text-3xl font-semibold tracking-tight text-slate-900 sm:text-4xl">
              Healthcare across Bangladesh, a few taps away
            </h1>
            <p className="mt-4 max-w-xl text-slate-600">
              Search verified doctors by name, specialty or district and book your serial without waiting in line.
            </p>
          </div>
          <Image
            src={heroImage}
            alt="A doctor in a white coat holding a phone, with healthcare icons rising from the screen"
            preload
            placeholder="blur"
            sizes="(min-width: 1024px) 480px, 100vw"
            className="w-full rounded-2xl object-cover shadow-lg ring-1 ring-slate-200"
          />
          <div className="lg:col-span-2">
            <DoctorSearchForm />
          </div>
        </Container>
      </section>

      <Container>
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-red-200 bg-red-50 px-5 py-4">
          <p className="text-sm text-red-800">
            <span className="font-semibold">Emergency?</span> Call{" "}
            <a href="tel:999" className="font-bold underline">
              999
            </a>{" "}
            for an ambulance, anywhere in Bangladesh.
          </p>
          <Link href="/hospitals#sylhet" className="text-sm font-medium text-red-800 underline">
            Sylhet hospital and ambulance numbers →
          </Link>
        </div>
      </Container>

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
              className="rounded-xl border border-slate-200 bg-surface p-5 shadow-sm transition hover:border-teal-600"
            >
              {body}
            </Link>
          ) : (
            <div key={s.title} className="rounded-xl border border-slate-200 bg-surface p-5 shadow-sm">
              {body}
            </div>
          );
        })}
      </Container>
    </>
  );
}
