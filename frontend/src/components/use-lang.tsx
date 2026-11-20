"use client";

import { useCallback, useSyncExternalStore } from "react";
import { LANG_STORAGE_KEY, translate, type Lang, type Vars } from "@/lib/i18n";

const listeners = new Set<() => void>();

function readLang(): Lang {
  return document.documentElement.getAttribute("data-lang") === "bn" ? "bn" : "en";
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function setLang(lang: Lang) {
  const d = document.documentElement;
  d.setAttribute("data-lang", lang);
  d.setAttribute("lang", lang);
  try {
    localStorage.setItem(LANG_STORAGE_KEY, lang);
  } catch {
    // private mode: the choice just isn't remembered
  }
  listeners.forEach((l) => l());
}

// The current language. The server can't know it, so it renders English and the
// client corrects it right after hydration; visible text should use <T>, which
// has no such delay.
export function useLang(): Lang {
  return useSyncExternalStore(subscribe, readLang, () => "en");
}

// t("Hello, {name}", { name }) for placeholders, labels, confirm() texts and the like.
export function useT() {
  const lang = useLang();
  return useCallback((text: string, vars?: Vars) => translate(lang, text, vars), [lang]);
}

export function LangToggle() {
  const lang = useLang();
  const next: Lang = lang === "bn" ? "en" : "bn";
  return (
    <button
      type="button"
      onClick={() => setLang(next)}
      className="grid h-8 min-w-8 place-items-center rounded-lg px-1.5 text-sm font-semibold text-white/80 hover:bg-white/10 hover:text-white"
      aria-label={next === "bn" ? "বাংলায় দেখুন (Switch to Bangla)" : "Switch to English"}
      title={next === "bn" ? "বাংলা" : "English"}
      lang={next}
    >
      {next === "bn" ? "বাং" : "EN"}
    </button>
  );
}
