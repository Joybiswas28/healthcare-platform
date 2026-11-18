// Browser location, wrapped in a promise with readable errors.

export type Coords = { latitude: number; longitude: number };

export function currentPosition(): Promise<Coords> {
  return new Promise((resolve, reject) => {
    if (typeof navigator === "undefined" || !navigator.geolocation) {
      reject(new Error("This browser can't share its location. Enter it by hand instead."));
      return;
    }
    navigator.geolocation.getCurrentPosition(
      (p) => resolve({ latitude: round(p.coords.latitude), longitude: round(p.coords.longitude) }),
      (err) =>
        reject(
          new Error(
            err.code === err.PERMISSION_DENIED
              ? "Location permission was refused. Allow it in the browser, or enter the location by hand."
              : "Couldn't get your location. Try again, or enter it by hand.",
          ),
        ),
      { enableHighAccuracy: true, timeout: 15000, maximumAge: 60000 },
    );
  });
}

function round(n: number): number {
  return Math.round(n * 1e6) / 1e6;
}

export function mapLink(latitude?: number | null, longitude?: number | null): string | null {
  if (latitude == null || longitude == null) return null;
  return `https://www.google.com/maps/search/?api=1&query=${latitude},${longitude}`;
}

export function km(n: number | undefined): string {
  if (n == null) return "";
  return n < 1 ? `${Math.round(n * 1000)} m` : `${n.toFixed(1)} km`;
}
