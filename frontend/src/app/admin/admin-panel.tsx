"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { RequireAuth } from "@/components/require-auth";
import { DoctorsQueue, HospitalsQueue, PharmaciesQueue, RidersQueue } from "./queues";
import { UsersTab } from "./users-tab";
import { WithdrawalsTab } from "./withdrawals-tab";

const TABS = [
  { id: "doctors", label: "Doctors" },
  { id: "hospitals", label: "Hospitals" },
  { id: "pharmacies", label: "Pharmacies" },
  { id: "riders", label: "Riders" },
  { id: "withdrawals", label: "Withdrawals" },
  { id: "users", label: "Accounts" },
] as const;
type Tab = (typeof TABS)[number]["id"];

export function AdminPanel() {
  return (
    <RequireAuth>
      {(user) => (user.role === "admin" ? <Panel /> : <p className="text-sm text-slate-600">This page is for platform admins.</p>)}
    </RequireAuth>
  );
}

function Panel() {
  const params = useSearchParams();
  const router = useRouter();
  const tab = (TABS.find((t) => t.id === params.get("tab"))?.id ?? "doctors") as Tab;

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-slate-900">Admin</h1>
      <div className="flex gap-1 overflow-x-auto border-b border-slate-200 [mask-image:linear-gradient(to_left,transparent,black_2.5rem)] md:[mask-image:none]" role="tablist">
        {TABS.map((t) => (
          <button
            key={t.id}
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => router.replace(`/admin?tab=${t.id}`, { scroll: false })}
            className={`whitespace-nowrap border-b-2 px-4 py-2 text-sm font-medium ${
              tab === t.id ? "border-teal-700 text-teal-800" : "border-transparent text-slate-600 hover:text-slate-900"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>
      {tab === "doctors" && <DoctorsQueue />}
      {tab === "hospitals" && <HospitalsQueue />}
      {tab === "pharmacies" && <PharmaciesQueue />}
      {tab === "riders" && <RidersQueue />}
      {tab === "withdrawals" && <WithdrawalsTab />}
      {tab === "users" && <UsersTab />}
    </div>
  );
}
