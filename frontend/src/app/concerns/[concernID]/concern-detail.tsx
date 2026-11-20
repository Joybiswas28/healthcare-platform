"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { RequireAuth } from "@/components/require-auth";
import { TextArea } from "@/components/textarea";
import { LoadFailure, loadFailure } from "@/components/load-failure";
import { Alert, Badge, Button, ButtonLink, Card, Spinner } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage, type Concern, type Offer, type User } from "@/lib/api/client";
import { dateTime, tokens } from "@/lib/format";
import { T } from "@/components/t";

export function ConcernDetail() {
  return <RequireAuth>{(user) => <Detail user={user} />}</RequireAuth>;
}

function Detail({ user }: { user: User }) {
  const t = useT();
  const { concernID } = useParams<{ concernID: string }>();
  const [concern, setConcern] = useState<Concern | null>(null);
  const [error, setError] = useState("");

  const reload = useCallback(async () => {
    const { data, error, response } = await api.GET("/api/v1/concerns/{concernID}", {
      params: { path: { concernID } },
    });
    if (data) setConcern(data);
    else setError(loadFailure(response.status, error, "concern"));
  }, [concernID]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload();
  }, [reload]);

  if (error && !concern) return <LoadFailure failure={error} backHref="/concerns" backLabel="Back to concerns" />;
  if (!concern) return <Spinner />;

  const isPatient = user.role === "patient";

  return (
    <div className="space-y-4">
      <Link href="/concerns" className="text-sm text-teal-800 hover:underline">
        ← {t(isPatient ? "My concerns" : "All concerns")}
      </Link>
      <Card>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <h1 className="text-xl font-semibold text-slate-900">{concern.title}</h1>
          {concern.specialization && <Badge>{concern.specialization}</Badge>}
        </div>
        <p className="text-xs text-slate-500">Posted {dateTime(concern.created_at)}</p>
        <p className="mt-4 whitespace-pre-line text-sm text-slate-700">{concern.description}</p>
      </Card>

      {isPatient ? (
        <PatientOffers concern={concern} onChange={setConcern} />
      ) : (
        <DoctorOffer concern={concern} onChange={reload} />
      )}
    </div>
  );
}

function PatientOffers({ concern, onChange }: { concern: Concern; onChange: (c: Concern) => void }) {
  const t = useT();
  const router = useRouter();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [needsTokens, setNeedsTokens] = useState(false);
  const offers = concern.offers ?? [];

  async function accept(o: Offer) {
    if (!o.id || !confirm(t("Accept {doctor}'s offer and pay {fee}? The chat starts right away.", { doctor: o.doctor_name, fee: tokens(o.fee_tokens) }))) return;
    setBusy(o.id);
    setError("");
    setNeedsTokens(false);
    const { data, error, response } = await api.POST("/api/v1/offers/{offerID}/accept", {
      params: { path: { offerID: o.id } },
    });
    setBusy(null);
    if (data?.id) {
      router.push(`/consultations/${data.id}`);
      return;
    }
    setNeedsTokens(response.status === 409);
    setError(errorMessage(error));
  }

  async function close() {
    if (!concern.id || !confirm(t("Close this concern? Pending offers will be declined."))) return;
    setBusy("close");
    const { data, error } = await api.POST("/api/v1/concerns/{concernID}/close", {
      params: { path: { concernID: concern.id } },
    });
    setBusy(null);
    if (data) onChange(data);
    else setError(errorMessage(error));
  }

  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold text-slate-900"><T>Offers from doctors</T></h2>
      {concern.status === "converted" && concern.consultation_id && (
        <Alert kind="success">
          <T>You accepted an offer.</T>{" "}
          <Link href={`/consultations/${concern.consultation_id}`} className="font-medium underline">
            <T>Open the consultation</T>
          </Link>
        </Alert>
      )}
      {concern.status === "closed" && <Alert kind="info">You closed this concern.</Alert>}
      {error && (
        <Alert>
          {error}
          {needsTokens && (
            <>
              {" "}
              <Link href="/wallet" className="font-medium underline">
                <T>Buy tokens</T>
              </Link>
            </>
          )}
        </Alert>
      )}
      {offers.length === 0 && concern.status === "open" && (
        <p className="text-sm text-slate-600"><T>No offers yet. Doctors will see your concern and respond here.</T></p>
      )}
      {offers.map((o) => (
        <Card key={o.id}>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <Link href={`/doctors/${o.doctor_id}`} className="font-semibold text-slate-900 hover:underline">
                {o.doctor_name}
              </Link>
              <p className="text-sm text-slate-600">
                {[o.specialization, o.degrees].filter(Boolean).join(" · ")}
                {o.experience_years ? ` · ${o.experience_years} years` : ""}
              </p>
            </div>
            <p className="text-sm font-semibold text-slate-900">{tokens(o.fee_tokens)}</p>
          </div>
          <p className="mt-3 whitespace-pre-line text-sm text-slate-700">{o.message}</p>
          <div className="mt-3 flex items-center justify-between gap-3">
            <span className="text-xs text-slate-500">{dateTime(o.created_at)}</span>
            {o.status === "pending" && concern.status === "open" ? (
              <Button onClick={() => accept(o)} disabled={busy !== null}>
                {busy === o.id ? "Accepting…" : "Accept and pay"}
              </Button>
            ) : (
              <Badge tone={o.status === "accepted" ? "teal" : "slate"}>{o.status}</Badge>
            )}
          </div>
        </Card>
      ))}
      {concern.status === "open" && (
        <Button variant="secondary" onClick={close} disabled={busy !== null}>
          <T>Close concern</T>
        </Button>
      )}
    </section>
  );
}

function DoctorOffer({ concern, onChange }: { concern: Concern; onChange: () => void }) {
  const t = useT();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const mine = concern.my_offer;

  async function send(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!concern.id) return;
    setBusy(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/concerns/{concernID}/offers", {
      params: { path: { concernID: concern.id } },
      body: { message: String(new FormData(e.currentTarget).get("message")).trim() },
    });
    setBusy(false);
    if (data) onChange();
    else setError(errorMessage(error));
  }

  async function withdraw() {
    if (!mine?.id || !confirm(t("Withdraw your offer?"))) return;
    setBusy(true);
    const { error } = await api.POST("/api/v1/offers/{offerID}/withdraw", { params: { path: { offerID: mine.id } } });
    setBusy(false);
    if (error) setError(errorMessage(error));
    else onChange();
  }

  if (mine && mine.status !== "withdrawn") {
    return (
      <Card>
        <h2 className="font-semibold text-slate-900"><T>Your offer</T></h2>
        <p className="mt-2 whitespace-pre-line text-sm text-slate-700">{mine.message}</p>
        <p className="mt-2 text-sm text-slate-600">
          {tokens(mine.fee_tokens)} · <Badge tone={mine.status === "accepted" ? "teal" : "slate"}>{mine.status}</Badge>
        </p>
        {error && <div className="mt-3"><Alert>{error}</Alert></div>}
        {mine.status === "pending" && (
          <Button variant="secondary" className="mt-3" onClick={withdraw} disabled={busy}>
            <T>Withdraw offer</T>
          </Button>
        )}
        {mine.status === "accepted" && (
          <ButtonLink href="/consultations" className="mt-3">
            <T>Go to consultations</T>
          </ButtonLink>
        )}
      </Card>
    );
  }

  if (concern.status !== "open") {
    return <Alert kind="info">This concern is no longer taking offers.</Alert>;
  }

  return (
    <Card>
      <form onSubmit={send} className="space-y-3">
        <h2 className="font-semibold text-slate-900"><T>Offer to help</T></h2>
        <TextArea
          label="Message to the patient"
          name="message"
          rows={4}
          minLength={10}
          maxLength={1000}
          required
          hint="Your current online consultation fee applies. Turn consultations on in your dashboard first."
        />
        {error && <Alert>{error}</Alert>}
        <Button type="submit" disabled={busy}>
          {busy ? "Sending…" : "Send offer"}
        </Button>
      </form>
    </Card>
  );
}
