import Image from "next/image";
import Link from "next/link";
import heroImage from "@/assets/hero-doctor.jpg";
import { DoctorSearchForm } from "@/components/doctor-search-form";
import { T } from "@/components/t";
import { Container } from "@/components/ui";

const SERVICES = [
  {
    title: "Book a doctor's serial",
    body: "Find a verified doctor, pick a chamber session and get your serial number and e-ticket instantly.",
    href: "/doctors",
  },
  {
    title: "Consult online",
    body: "Chat with a doctor from home using consultation tokens, or post a concern and receive offers.",
    href: "/consultations",
  },
  {
    title: "Hospitals",
    body: "Browse hospitals, their doctors and chambers, and book through the hospital directly.",
    href: "/hospitals",
  },
  {
    title: "Medicine delivery",
    body: "Send your prescription to nearby pharmacies, compare quotes and get medicine delivered.",
    href: "/medicines/new",
  },
];

export default function Home() {
  return (
    <>
      <section className="bg-gradient-to-b from-teal-50 to-transparent">
        <Container className="grid items-center gap-8 py-12 sm:py-16 lg:grid-cols-2 lg:gap-12">
          <div>
            <p className="text-sm font-semibold uppercase tracking-wider text-teal-800">
              <T>Digital Healthcare Platform</T>
            </p>
            <h1 className="mt-2 max-w-2xl text-3xl font-semibold tracking-tight text-slate-900 sm:text-4xl">
              <T>Healthcare across Bangladesh, a few taps away</T>
            </h1>
            <p className="mt-4 max-w-xl text-slate-600">
              <T>Search verified doctors by name, specialty or district and book your serial without waiting in line.</T>
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
          <div className="flex flex-wrap items-center gap-3">
            <a
              href="tel:999"
              className="rounded-lg bg-red-600 px-3 py-1.5 text-lg font-bold text-white hover:bg-red-500"
              aria-label="Call 999"
            >
              📞 999
            </a>
            <p className="text-sm text-red-800">
              <span className="font-semibold">
                <T>Emergency?</T>
              </span>{" "}
              <T>Call 999 for an ambulance, anywhere in Bangladesh.</T>
            </p>
          </div>
          <Link href="/hospitals#sylhet" className="text-sm font-medium text-red-800 underline">
            <T>Sylhet hospital and ambulance numbers →</T>
          </Link>
        </div>
      </Container>

      <Container className="grid gap-4 py-12 sm:grid-cols-2">
        {SERVICES.map((s) => (
          <Link
            key={s.title}
            href={s.href}
            className="rounded-xl border border-slate-200 bg-surface p-5 shadow-sm transition hover:border-teal-600"
          >
            <h2 className="font-semibold text-slate-900">
              <T>{s.title}</T>
            </h2>
            <p className="mt-2 text-sm text-slate-600">
              <T>{s.body}</T>
            </p>
          </Link>
        ))}
      </Container>
    </>
  );
}
