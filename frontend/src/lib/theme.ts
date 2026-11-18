// Shared by the <head> script (layout) and the toggle button (client).
export type Theme = "light" | "dark";
export const THEME_STORAGE_KEY = "theme";

// Runs in <head> before the first paint: the saved choice, else the device setting.
export const THEME_SCRIPT = `(function(){try{var t=localStorage.getItem("${THEME_STORAGE_KEY}");if(t!=="light"&&t!=="dark"){t=window.matchMedia("(prefers-color-scheme: dark)").matches?"dark":"light"}document.documentElement.setAttribute("data-theme",t)}catch(e){document.documentElement.setAttribute("data-theme","light")}})()`;
