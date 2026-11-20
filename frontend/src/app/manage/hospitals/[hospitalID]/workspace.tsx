"use client";

import Link from "next/link";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { RequireAuth } from "@/components/require-auth";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { Alert, Badge, Spinner } from "@/components/ui";
import { api, type Hospital } from "@/lib/api/client";
import { VERIFICATION } from "../my-hospitals";
import { BookingsTab } from "./bookings-tab";
import { DoctorsTab } from "./doctors-tab";
import { MembersTab } from "./members-tab";
import { ProfileTab } from "./profile-tab";
import { T } from "@/components/t";

const TABS = [
  { id: "doctors", label: "Doctors & sessions" },
  { id: "bookings", label: "Bookings" },
  { id: "profile", label: "Profile & photos" },
  { id: "team", label: "Team" },
] as const;

type Tab = (typeof TABS)[number]["id"];

export function Workspace() {
  return (
    <RequireAuth>
      {(user) =>
        user.role === "hospital_admin" ? <Inner /> : <p className="text-sm text-slate-600"><T>This page is for hospital administrators.</T></p>
      }
    </RequireAuth>
  );
}

function Inner() {
  const { hospitalID } = useParams<{ hospitalID: string }>();
  const params = useSearchParams();
  const router = useRouter();
  const tab = (TABS.find((t) => t.id === params.get("tab"))?.id ?? "doctors") as Tab;
  const [hospital, setHospital] = useState<Hospital | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api.GET("/api/v1/manage/hospitals/{hospitalID}", { params: { path: { hospitalID } } }).then(({ data, error, response }) => {
      if (data) setHospital(data);
      else setError(loadFailure(response.status, error, "hospital"));
    });
  }, [hospitalID]);

  if (error) return <LoadFailure failure={error} backHref="/manage/hospitals" backLabel="My hospitals" />;
  if (!hospital) return <Spinner />;

  const v = VERIFICATION[hospital.verification_status ?? "pending"];

  return (
    <div className="space-y-6">
      <Link href="/manage/hospitals" className="text-sm text-teal-800 hover:underline">
        ← <T>My hospitals</T>
      </Link>
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-semibold text-slate-900">{hospital.name}</h1>
        <Badge tone={v.tone}>{v.label}</Badge>
        {hospital.verification_status === "approved" && (
          <Link href={`/hospitals/${hospital.id}`} className="text-sm text-teal-800 hover:underline">
            <T>View public page</T>
          </Link>
        )}
      </div>
      {hospital.verification_status === "rejected" && hospital.verification_note && (
        <Alert>
          <T vars={{ note: hospital.verification_note }}>
            {"Verification was rejected: {note}. Correct the profile and save it to ask for another check."}
          </T>
        </Alert>
      )}
      {hospital.verification_status === "pending" && (
        <Alert kind="info">
          <T>
            {"An admin is checking your licence number. You can prepare your profile and photos meanwhile; inviting doctors opens once the hospital is verified."}
          </T>
        </Alert>
      )}

      <div className="flex gap-1 overflow-x-auto border-b border-slate-200 [mask-image:linear-gradient(to_left,transparent,black_2.5rem)] md:[mask-image:none]" role="tablist">
        {TABS.map((t) => (
          <button
            key={t.id}
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => router.replace(`/manage/hospitals/${hospitalID}?tab=${t.id}`, { scroll: false })}
            className={`whitespace-nowrap border-b-2 px-4 py-2 text-sm font-medium ${
              tab === t.id ? "border-teal-700 text-teal-800" : "border-transparent text-slate-600 hover:text-slate-900"
            }`}
          >
            <T>{t.label}</T>
          </button>
        ))}
      </div>

      {tab === "doctors" && <DoctorsTab hospital={hospital} />}
      {tab === "bookings" && <BookingsTab hospital={hospital} />}
      {tab === "profile" && <ProfileTab hospital={hospital} onChange={setHospital} />}
      {tab === "team" && <MembersTab hospital={hospital} />}
    </div>
  );
}
