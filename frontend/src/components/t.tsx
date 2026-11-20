import { BN } from "@/lib/i18n-bn";
import { interpolate, type Vars } from "@/lib/i18n";

// Translated text that works in server and client components without a flash:
// both languages are rendered and CSS (globals.css) shows the one matching
// <html data-lang>, which the head script sets before the first paint.
// Use it for visible text; for attributes (placeholder, aria-label) and
// strings in code, use useT() instead.
//
// bnVars overrides values for the Bangla text, e.g. a date already formatted
// in Bangla by a server component.
export function T({ children, vars, bnVars }: { children: string; vars?: Vars; bnVars?: Vars }) {
  const en = interpolate(children, vars);
  const bn = BN[children];
  if (bn === undefined) return <>{en}</>;
  return (
    <>
      <span data-l="en">{en}</span>
      <span data-l="bn">{interpolate(bn, bnVars ? { ...vars, ...bnVars } : vars)}</span>
    </>
  );
}
