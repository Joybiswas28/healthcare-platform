"use client";

import { useEffect } from "react";
import { Button, ButtonLink, Card, Container } from "@/components/ui";

// Catches crashes inside a page. The header and footer stay, so people can
// keep using the rest of the site.
export default function PageError({ error, retry }: { error: unknown; retry: () => void }) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  // In production, server errors carry only a reference code (digest), never details.
  const digest = error instanceof Error && "digest" in error ? String((error as Error & { digest?: string }).digest ?? "") : "";

  return (
    <Container className="py-16">
      <Card className="mx-auto max-w-lg text-center">
        <p className="text-4xl" aria-hidden>
          ⚠️
        </p>
        <h1 className="mt-2 text-xl font-semibold text-slate-900">Something went wrong</h1>
        <p className="mt-2 text-sm text-slate-600">
          This page hit an unexpected problem. Trying again usually helps; your data is safe.
        </p>
        {digest && <p className="mt-2 font-mono text-xs text-slate-500">Reference: {digest}</p>}
        <div className="mt-5 flex flex-wrap justify-center gap-2">
          <Button onClick={() => retry()}>Try again</Button>
          <ButtonLink href="/" variant="secondary">
            Home
          </ButtonLink>
        </div>
      </Card>
    </Container>
  );
}
