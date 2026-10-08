"use client";

import { useEffect, useState } from "react";

type ConsentView = {
  state: "PENDING" | "AUTHENTICATED" | "DENIED";
  client_id?: string;
  client_name?: string;
  resource?: string;
  scope?: string;
  user_id?: string;
  csrf: string;
  authentication_available?: boolean;
};

export function OnboardingUI() {
  const [view, setView] = useState<ConsentView | null>(null);
  const [message, setMessage] = useState("Connecting to WizPay…");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    fetch("/browser/session", { credentials: "same-origin", cache: "no-store", signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw new Error("Your connection request is unavailable or has expired. Restart from your AI client.");
        const data: ConsentView = await response.json();
        setView(data);
        setMessage(data.state === "DENIED" ? "Authorization denied. No MCP access was granted." : "");
      })
      .catch(error => { if (error.name !== "AbortError") setMessage("Your connection request is unavailable or has expired. Restart from your AI client."); });
    return () => controller.abort();
  }, []);
  async function decide(decision: "grant" | "deny") {
    if (!view || busy) return;
    setBusy(true);
    try {
      const response = await fetch("/browser/consent", {
        method: "POST", credentials: "same-origin", cache: "no-store",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": view.csrf },
        body: JSON.stringify({ decision }),
      });
      if (!response.ok) throw new Error();
      // WP3A has no verified authentication provider. Grant stays unavailable;
      // only cancellation is operational. Never manufacture a success state.
      if (decision === "deny") { setView({ ...view, state: "DENIED" }); setMessage("Authorization denied. No MCP access was granted."); }
      else setMessage("Authorization completion is unavailable. Restart from your AI client.");
    } catch { setMessage("The request could not be completed. Restart from your AI client if it has expired."); }
    finally { setBusy(false); }
  }
  return (
    <main className="mx-auto flex min-h-screen max-w-2xl flex-col justify-center px-5 py-12">
      <div className="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm sm:p-10">
        <p className="text-sm font-semibold tracking-widest text-blue-700">WIZPAY MCP</p>
        <h1 className="mt-3 text-3xl font-bold">Connect your AI client</h1>
        <p className="mt-3 text-slate-600">Review the application requesting access to your WizPay information.</p>
        <p role="status" aria-live="polite" className="mt-4 text-slate-700">{message}</p>
        {view && view.state !== "DENIED" && <>
          <dl className="mt-6 grid gap-4 rounded-xl bg-slate-50 p-5">
            <div><dt className="text-sm text-slate-500">Registered application</dt><dd className="font-semibold">{view.client_name}</dd><dd className="break-all text-sm">{view.client_id}</dd></div>
            <div><dt className="text-sm text-slate-500">Protected resource</dt><dd className="break-all">{view.resource}</dd></div>
            <div><dt className="text-sm text-slate-500">Requested permissions</dt><dd>{view.scope}: read intent and approval status</dd></div>
            {view.user_id && <div><dt className="text-sm text-slate-500">Authenticated user</dt><dd className="break-all">{view.user_id}</dd></div>}
          </dl>
          <p className="mt-5 text-sm text-slate-600">This access does not authorize fund transfers, wallet signing, or financial approvals.</p>
          <div className="mt-5 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-950">Wallet authentication is not available yet. This request remains unauthenticated and cannot grant MCP access.</div>
          <div className="mt-6 flex flex-col gap-3 sm:flex-row">
            <button disabled aria-describedby="grant-note" className="rounded-xl bg-slate-200 px-5 py-3 font-semibold text-slate-500">Grant read-only access</button>
            <button disabled={busy} onClick={() => decide("deny")} className="rounded-xl border border-slate-300 px-5 py-3 font-semibold focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-blue-600">{busy ? "Cancelling…" : "Deny access"}</button>
          </div>
          <p id="grant-note" className="mt-3 text-sm text-slate-500">Grant becomes available only after verified wallet authentication is implemented.</p>
        </>}
      </div>
    </main>
  );
}
