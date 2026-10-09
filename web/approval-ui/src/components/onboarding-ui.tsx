"use client";

import { useEffect, useState } from "react";

import { browserAPI } from "../lib/onboarding-api";
import { Flow, type State } from "../lib/onboarding-flow";
import { discover, type Wallet } from "../lib/wallet/provider";

export function OnboardingUI() {
  const [flow, setFlow] = useState<Flow | null>(null);
  const [state, setState] = useState<State>({ phase: "initializing", message: "Initializing authorization…", busy: false, restart: false, accounts: [] });
  const [wallets, setWallets] = useState<Wallet[]>([]);
  useEffect(() => {
    const controller = new Flow(browserAPI(), setState, url => window.location.assign(url));
    setFlow(controller);
    const cleanup = discover(window, setWallets);
    void controller.initialize();
    return () => { cleanup(); controller.dispose(); };
  }, []);
  const view = state.view;
  const busy = state.busy;
  const message = state.message;
  const disabled = busy || state.restart;
  const buttonStyle = "rounded-xl border border-slate-300 px-5 py-3 font-semibold disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-blue-600";
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
          {view.state === "PENDING" && <section aria-label="Wallet authentication" className="mt-5 grid gap-4">
            <p className="text-sm text-slate-600">Only existing reviewed external wallet bindings can authenticate. No account is created. QR pairing and contract-wallet signatures are unsupported.</p>
            {!view.authentication_available && <p role="status">Wallet authentication is unavailable in this environment.</p>}
            {wallets.length === 0 && <p>No supported injected wallet found. Use a compatible extension or wallet browser.</p>}
            <label className="grid gap-2">Wallet
              <select className="rounded-lg border p-3" disabled={disabled || !view.authentication_available} value={state.wallet?.id ?? ""} onChange={e => { const wallet = wallets.find(w => w.id === e.target.value); if (wallet) flow?.select(wallet); }}>
                <option value="" disabled>Select a wallet</option>
                {wallets.map(wallet => <option key={wallet.id} value={wallet.id}>{wallet.name}</option>)}
              </select>
            </label>
            <button className={buttonStyle} disabled={disabled || !state.wallet || !view.authentication_available} onClick={() => void flow?.connect()}>Connect wallet</button>
            {state.accounts.length > 0 && <label className="grid gap-2">Authentication account
              <select className="rounded-lg border p-3" disabled={disabled} value={state.account ?? ""} onChange={e => flow?.chooseAccount(e.target.value)}>
                <option value="" disabled>Select an account</option>
                {state.accounts.map(account => <option key={account} value={account}>{account}</option>)}
              </select>
            </label>}
            {state.account && <p className="break-all text-sm">Selected wallet: {state.account}. Required network: Arc Mainnet (5042). Connected does not mean authenticated.</p>}
            <button className={buttonStyle} disabled={disabled || !state.account || !view.authentication_available} onClick={() => void flow?.requestChallenge()}>Request authentication message</button>
            {state.challenge && <>
              <p>Review the exact authentication message. This does not authorize transactions.</p>
              <pre className="overflow-auto whitespace-pre-wrap break-words rounded-xl bg-slate-50 p-4 text-sm">{state.challenge.message}</pre>
              <p className="text-sm">Expires: {state.challenge.expires_at}</p>
              <button className={buttonStyle} disabled={disabled} onClick={() => void flow?.authenticate()}>Sign message to authenticate</button>
            </>}
          </section>}
          <div className="mt-6 flex flex-col gap-3 sm:flex-row">
            <button disabled={!flow?.canGrant} onClick={() => void flow?.decide("grant")} aria-describedby="grant-note" className={buttonStyle + " bg-blue-700 text-white"}>Grant read-only access</button>
            <button disabled={disabled} onClick={() => void flow?.decide("deny")} className="rounded-xl border border-slate-300 px-5 py-3 font-semibold focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-blue-600">{busy ? "Please wait…" : "Deny access"}</button>
          </div>
          <p id="grant-note" className="mt-3 text-sm text-slate-500">Grant requires verified server-side wallet authentication and a separate explicit consent decision.</p>
        </>}
      </div>
    </main>
  );
}
