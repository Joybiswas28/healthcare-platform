"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api, type FamilyMember } from "@/lib/api/client";
import { bnDigits } from "@/lib/i18n";
import { T } from "./t";
import { useLang, useT } from "./use-lang";

export const RELATIONS: Record<string, string> = {
  spouse: "Spouse",
  child: "Child",
  parent: "Parent",
  sibling: "Brother or sister",
  other: "Other relative",
};

// Several booking panels can be on one page; they share one request.
let cache: Promise<FamilyMember[]> | null = null;

export function forgetFamily() {
  cache = null;
}

// The patient's family members, or null while loading.
export function useFamily(enabled = true): FamilyMember[] | null {
  const [members, setMembers] = useState<FamilyMember[] | null>(null);
  useEffect(() => {
    if (!enabled) return;
    cache ??= api.GET("/api/v1/family").then(({ data }) => data?.members ?? []);
    let cancelled = false;
    cache.then((m) => {
      if (!cancelled) setMembers(m);
    });
    return () => {
      cancelled = true;
    };
  }, [enabled]);
  return members;
}

// "Who is this for?" — the account holder or one of their family members.
// value "" means the account holder.
export function PersonPicker({
  members,
  me,
  value,
  onChange,
}: {
  members: FamilyMember[];
  me: string;
  value: string;
  onChange: (id: string) => void;
}) {
  const t = useT();
  return (
    <div className="text-sm">
      <label className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-slate-700">
          <T>Who is this for?</T>
        </span>
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="rounded-lg border border-slate-300 bg-surface px-2 py-1.5 text-sm outline-none focus:border-teal-600"
        >
          <option value="">{t("Myself ({name})", { name: me })}</option>
          {members.map((m) => (
            <option key={m.id} value={m.id}>
              {m.name} ({t(RELATIONS[m.relation ?? ""] ?? "")})
            </option>
          ))}
        </select>
      </label>
      {members.length === 0 && (
        <p className="mt-1 text-xs text-slate-500">
          <T>Booking for a child or parent?</T>{" "}
          <Link href="/family" className="font-medium text-teal-800 hover:underline">
            <T>Add family members</T>
          </Link>
        </p>
      )}
    </div>
  );
}

// Whole years from a YYYY-MM-DD birth date to today.
export function ageYears(birth: string | null | undefined): number | null {
  if (!birth) return null;
  const [y, m, d] = birth.split("-").map(Number);
  const now = new Date();
  let age = now.getFullYear() - y;
  if (now.getMonth() + 1 < m || (now.getMonth() + 1 === m && now.getDate() < d)) age--;
  return age >= 0 ? age : null;
}

// "Rahim · child, 6 y" for a booking made for a family member; nothing for
// the account holder's own bookings.
export function ForMember({
  relation,
  birthDate,
  accountName,
  showAccount = false,
}: {
  relation?: string | null;
  birthDate?: string | null;
  accountName?: string;
  showAccount?: boolean;
}) {
  const t = useT();
  const lang = useLang();
  if (!relation) return null;
  const age = ageYears(birthDate);
  const parts = [t(RELATIONS[relation] ?? relation)];
  if (age != null) parts.push(t("{n} years", { n: age }));
  if (showAccount && accountName) parts.push(t("booked by {name}", { name: accountName }));
  const text = parts.join(" · ");
  return (
    <span className="inline-block rounded-full bg-sky-50 px-2 py-0.5 text-xs font-medium text-sky-800">
      👪 {lang === "bn" ? bnDigits(text) : text}
    </span>
  );
}
