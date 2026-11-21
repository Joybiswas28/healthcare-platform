"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { T } from "@/components/t";
import { Button } from "@/components/ui";
import { useT } from "@/components/use-lang";
import { api, errorMessage } from "@/lib/api/client";

// A video call between the patient and the doctor of an active consultation.
// Audio and video go browser to browser (WebRTC); the API only relays the
// setup messages. The caller sends an offer, the other side answers, and both
// trickle network candidates until the connection is up.

type Phase = "idle" | "calling" | "incoming" | "connecting" | "live";
type Signal = { id?: number; kind?: string; payload?: string | null };

const RING_MS = 60_000;

export function VideoCall({ consultationID, otherName }: { consultationID: string; otherName: string }) {
  const t = useT();
  const [phase, setPhase] = useState<Phase>("idle");
  const [notice, setNotice] = useState("");
  const [micOn, setMicOn] = useState(true);
  const [camOn, setCamOn] = useState(true);
  const [audioOnly, setAudioOnly] = useState(false);

  const phaseRef = useRef<Phase>("idle");
  const iceServers = useRef<RTCIceServer[]>([]);
  const pc = useRef<RTCPeerConnection | null>(null);
  const local = useRef<MediaStream | null>(null);
  const offer = useRef<RTCSessionDescriptionInit | null>(null);
  const pending = useRef<RTCIceCandidateInit[]>([]);
  const ringTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const localVideo = useRef<HTMLVideoElement>(null);
  const remoteVideo = useRef<HTMLVideoElement>(null);

  const go = useCallback((p: Phase) => {
    phaseRef.current = p;
    setPhase(p);
  }, []);

  const send = useCallback(
    async (kind: "offer" | "answer" | "candidate" | "hangup" | "decline", payload?: unknown) => {
      const { error, response } = await api.POST("/api/v1/consultations/{consultationID}/call/signals", {
        params: { path: { consultationID } },
        body: { kind, payload: payload === undefined ? undefined : JSON.stringify(payload) },
      });
      if (!response.ok) throw new Error(errorMessage(error));
    },
    [consultationID],
  );

  // Stop the camera and close the connection; keep the page where it is.
  const cleanUp = useCallback(
    (message = "") => {
      if (ringTimer.current) clearTimeout(ringTimer.current);
      ringTimer.current = null;
      pc.current?.close();
      pc.current = null;
      local.current?.getTracks().forEach((tr) => tr.stop());
      local.current = null;
      offer.current = null;
      pending.current = [];
      setMicOn(true);
      setCamOn(true);
      setAudioOnly(false);
      setNotice(message);
      go("idle");
    },
    [go],
  );

  async function getMedia(): Promise<MediaStream> {
    if (!navigator.mediaDevices?.getUserMedia) {
      throw new Error(t("This browser can't make video calls. Open the site over https in Chrome, Edge, Firefox or Safari."));
    }
    try {
      return await navigator.mediaDevices.getUserMedia({ video: { facingMode: "user" }, audio: true });
    } catch {
      // No camera, or camera refused: fall back to a voice call.
      const s = await navigator.mediaDevices.getUserMedia({ audio: true }).catch(() => {
        throw new Error(t("Allow the microphone (and camera) for this site to make calls."));
      });
      setAudioOnly(true);
      return s;
    }
  }

  function connect(stream: MediaStream): RTCPeerConnection {
    const conn = new RTCPeerConnection({ iceServers: iceServers.current });
    stream.getTracks().forEach((tr) => conn.addTrack(tr, stream));
    conn.onicecandidate = (e) => {
      if (e.candidate) send("candidate", e.candidate.toJSON()).catch(() => {});
    };
    conn.ontrack = (e) => {
      if (remoteVideo.current && remoteVideo.current.srcObject !== e.streams[0]) remoteVideo.current.srcObject = e.streams[0];
    };
    conn.onconnectionstatechange = () => {
      if (conn.connectionState === "connected") {
        if (ringTimer.current) clearTimeout(ringTimer.current);
        go("live");
      } else if (conn.connectionState === "failed") {
        send("hangup").catch(() => {});
        cleanUp(t("The call could not connect. One of you may be on a network that blocks direct calls; try another network or Wi-Fi."));
      }
    };
    pc.current = conn;
    return conn;
  }

  async function flushCandidates() {
    for (const c of pending.current.splice(0)) await pc.current?.addIceCandidate(c).catch(() => {});
  }

  async function startCall() {
    setNotice("");
    try {
      const stream = await getMedia();
      local.current = stream;
      go("calling");
      const conn = connect(stream);
      const desc = await conn.createOffer();
      await conn.setLocalDescription(desc);
      await send("offer", desc);
      ringTimer.current = setTimeout(() => {
        if (phaseRef.current === "calling") {
          send("hangup").catch(() => {});
          cleanUp(t("No answer. Try again later or send a message."));
        }
      }, RING_MS);
    } catch (e) {
      cleanUp(e instanceof Error ? e.message : String(e));
    }
  }

  async function accept() {
    const o = offer.current;
    if (!o) return;
    setNotice("");
    try {
      const stream = await getMedia();
      local.current = stream;
      go("connecting");
      const conn = connect(stream);
      await conn.setRemoteDescription(o);
      await flushCandidates();
      const answer = await conn.createAnswer();
      await conn.setLocalDescription(answer);
      await send("answer", answer);
    } catch (e) {
      send("hangup").catch(() => {});
      cleanUp(e instanceof Error ? e.message : String(e));
    }
  }

  function decline() {
    send("decline").catch(() => {});
    cleanUp();
  }

  function hangUp() {
    send("hangup").catch(() => {});
    cleanUp(t("Call ended."));
  }

  function toggle(kind: "audio" | "video") {
    const tracks = kind === "audio" ? local.current?.getAudioTracks() : local.current?.getVideoTracks();
    const on = !(tracks?.[0]?.enabled ?? false);
    tracks?.forEach((tr) => (tr.enabled = on));
    if (kind === "audio") setMicOn(on);
    else setCamOn(on);
  }

  // Signals from the other side.
  const onSignal = useCallback(
    async (s: Signal) => {
      const data = s.payload ? JSON.parse(s.payload) : null;
      const p = phaseRef.current;
      switch (s.kind) {
        case "offer":
          if (p === "idle") {
            offer.current = data;
            pending.current = [];
            setNotice("");
            go("incoming");
          }
          break;
        case "answer":
          if (p === "calling" && pc.current) {
            go("connecting");
            await pc.current.setRemoteDescription(data);
            await flushCandidates();
          }
          break;
        case "candidate":
          if (pc.current?.remoteDescription) await pc.current.addIceCandidate(data).catch(() => {});
          else pending.current.push(data);
          break;
        case "hangup":
          if (p !== "idle") cleanUp(p === "incoming" ? t("Missed call from {name}.", { name: otherName }) : t("Call ended."));
          break;
        case "decline":
          if (p === "calling") cleanUp(t("{name} can't take the call now.", { name: otherName }));
          break;
      }
    },
    [cleanUp, go, otherName, t],
  );

  // Settings, and the stream of signals, for as long as the room is open.
  useEffect(() => {
    api.GET("/api/v1/consultations/{consultationID}/call", { params: { path: { consultationID } } }).then(({ data }) => {
      iceServers.current = (data?.ice_servers ?? []).map((s) => ({ urls: s.urls ?? [], username: s.username, credential: s.credential }));
    });
    const es = new EventSource(`/api/v1/consultations/${consultationID}/call/events`);
    // Handle signals one at a time, in order.
    let chain = Promise.resolve();
    es.addEventListener("signal", (e) => {
      const s = JSON.parse((e as MessageEvent).data) as Signal;
      chain = chain.then(() => onSignal(s)).catch(() => {});
    });
    return () => es.close();
  }, [consultationID, onSignal]);

  // End the call if the page is closed mid-call.
  useEffect(() => {
    const bye = () => {
      if (phaseRef.current !== "idle") {
        navigator.sendBeacon?.(
          `/api/v1/consultations/${consultationID}/call/signals`,
          new Blob([JSON.stringify({ kind: "hangup" })], { type: "application/json" }),
        );
      }
    };
    window.addEventListener("pagehide", bye);
    return () => {
      window.removeEventListener("pagehide", bye);
      pc.current?.close();
      local.current?.getTracks().forEach((tr) => tr.stop());
    };
  }, [consultationID]);

  // Show the local camera once the stream exists.
  useEffect(() => {
    if (localVideo.current && local.current && localVideo.current.srcObject !== local.current) localVideo.current.srcObject = local.current;
  });

  const inCall = phase === "calling" || phase === "connecting" || phase === "live";

  return (
    <section className="rounded-xl border border-slate-200 bg-surface p-4 shadow-sm" aria-live="polite">
      {phase === "idle" && (
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="font-medium text-slate-900">
              📹 <T>Video call</T>
            </p>
            <p className="text-xs text-slate-500">
              <T>Talk face to face. The call goes directly between your two devices.</T>
            </p>
          </div>
          <Button onClick={startCall}>Start video call</Button>
          {notice && <p className="w-full text-sm text-slate-700">{notice}</p>}
        </div>
      )}

      {phase === "incoming" && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-teal-50 p-3">
          <p className="font-medium text-teal-900">
            📞 <T vars={{ name: otherName }}>{"{name} is calling you…"}</T>
          </p>
          <div className="flex gap-2">
            <Button onClick={accept}>Accept</Button>
            <Button variant="danger" onClick={decline}>
              Decline
            </Button>
          </div>
        </div>
      )}

      {inCall && (
        <div className="space-y-3">
          <p className="text-sm font-medium text-slate-700">
            {phase === "calling" ? (
              <T vars={{ name: otherName }}>{"Calling {name}…"}</T>
            ) : phase === "connecting" ? (
              <T>Connecting…</T>
            ) : (
              <T vars={{ name: otherName }}>{"In call with {name}"}</T>
            )}
          </p>
          <div className="relative overflow-hidden rounded-lg bg-slate-900">
            <video ref={remoteVideo} autoPlay playsInline className="aspect-[3/4] w-full object-cover sm:aspect-video" />
            {!audioOnly && (
              <video
                ref={localVideo}
                autoPlay
                playsInline
                muted
                className="absolute bottom-3 right-3 w-24 rounded-md border-2 border-white/70 object-cover shadow sm:w-36"
              />
            )}
            {phase !== "live" && (
              <p className="absolute inset-0 grid place-items-center text-sm text-white/80">
                <T>Waiting for the other side…</T>
              </p>
            )}
          </div>
          <div className="flex flex-wrap justify-center gap-2">
            <Button variant="secondary" onClick={() => toggle("audio")} aria-pressed={!micOn}>
              {micOn ? "🎙️ Mute" : "🔇 Unmute"}
            </Button>
            {!audioOnly && (
              <Button variant="secondary" onClick={() => toggle("video")} aria-pressed={!camOn}>
                {camOn ? "📷 Camera off" : "📷 Camera on"}
              </Button>
            )}
            <Button variant="danger" onClick={hangUp}>
              Hang up
            </Button>
          </div>
        </div>
      )}
    </section>
  );
}
