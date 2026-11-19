import type { ReactNode } from "react";
import { errorMessage } from "@/lib/api/client";
import { Alert, ButtonLink, Card } from "./ui";

// Detail pages load one record by the id in the URL. When that fails they keep
// a string in their error state; loadFailure() marks "not found" (404, or 422
// for a malformed id such as /doctors/abc) so <LoadFailure> can show a friendly
// card with a way back instead of a raw API message.

const MISSING = "\u0000missing:";

export function loadFailure(status: number, error: unknown, what: string): string {
  if (status === 404 || status === 422) return MISSING + what;
  return errorMessage(error);
}

export function LoadFailure({
  failure,
  backHref = "/",
  backLabel = "Go to the home page",
}: {
  failure: string;
  backHref?: string;
  backLabel?: string;
}) {
  if (!failure.startsWith(MISSING)) return <Alert>{failure}</Alert>;
  const what = failure.slice(MISSING.length);
  return (
    <NotFoundCard title={`This ${what} could not be found`} backHref={backHref} backLabel={backLabel}>
      The link may be wrong or out of date, or it may belong to another account.
    </NotFoundCard>
  );
}

export function NotFoundCard({
  title,
  children,
  backHref = "/",
  backLabel = "Go to the home page",
}: {
  title: string;
  children?: ReactNode;
  backHref?: string;
  backLabel?: string;
}) {
  return (
    <Card className="mx-auto max-w-lg text-center">
      <p className="text-4xl" aria-hidden>
        🔍
      </p>
      <h1 className="mt-2 text-xl font-semibold text-slate-900">{title}</h1>
      {children && <p className="mt-2 text-sm text-slate-600">{children}</p>}
      <div className="mt-5 flex flex-wrap justify-center gap-2">
        <ButtonLink href={backHref}>{backLabel}</ButtonLink>
        {backHref !== "/" && (
          <ButtonLink href="/" variant="secondary">
            Home
          </ButtonLink>
        )}
      </div>
    </Card>
  );
}
