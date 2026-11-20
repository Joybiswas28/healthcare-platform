// Two languages: English (the source text) and Bangla. Translations live in
// i18n-bn.ts, keyed by the English text, so an untranslated string simply stays
// English. Works on the server and the client.
import { BN } from "./i18n-bn";

export type Lang = "en" | "bn";
export const LANG_STORAGE_KEY = "lang";

// Runs in <head> before the first paint, like the theme script: the saved choice,
// else Bangla for browsers set to Bangla, else English.
export const LANG_SCRIPT = `(function(){try{var l=localStorage.getItem("${LANG_STORAGE_KEY}");if(l!=="en"&&l!=="bn"){l=(navigator.language||"").toLowerCase().indexOf("bn")===0?"bn":"en"}var d=document.documentElement;d.setAttribute("data-lang",l);d.setAttribute("lang",l)}catch(e){}})()`;

export type Vars = Record<string, string | number | undefined | null>;

// "Hello, {name}" + { name: "Karim" } -> "Hello, Karim"
export function interpolate(text: string, vars?: Vars): string {
  if (!vars) return text;
  return text.replace(/\{(\w+)\}/g, (m, k: string) => (vars[k] == null ? m : String(vars[k])));
}

export function hasBangla(text: string): boolean {
  return text in BN;
}

export function translate(lang: Lang, text: string, vars?: Vars): string {
  return interpolate(lang === "bn" ? (BN[text] ?? text) : text, vars);
}

// Bangla digits for numbers shown in Bangla text.
const BN_DIGITS = "০১২৩৪৫৬৭৮৯";
export function bnDigits(s: string): string {
  return s.replace(/[0-9]/g, (d) => BN_DIGITS[Number(d)]);
}
