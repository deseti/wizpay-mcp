"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import {
  Approval,
  ApprovalApiError,
  ApprovalDecision,
  decideApproval,
  authorizeExecution,
  getApproval,
  listConfiguredApprovals,
} from "@/lib/api";

function formatDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? "Unavailable" : date.toLocaleString();
}

function isExpired(approval: Approval) {
  return approval.status === "EXPIRED" || new Date(approval.expires_at).getTime() <= Date.now();
}

function errorLabel(error: unknown) {
  if (error instanceof ApprovalApiError && (error.status === 401 || error.status === 403)) {
    return "You are not authorized to view this approval.";
  }
  return "We could not complete the request. Try again later.";
}

function Status({ approval }: { approval: Approval }) {
  const expired = isExpired(approval);
  const label = expired ? "EXPIRED" : approval.status;
  return (
    <span className={`rounded-full px-2.5 py-1 text-xs font-semibold tracking-wide ${expired ? "bg-amber-100 text-amber-800" : "bg-blue-100 text-blue-800"}`}>
      {label}
    </span>
  );
}

export function ApprovalList() {
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();

  useEffect(() => {
    listConfiguredApprovals()
      .then(setApprovals)
      .catch((reason: unknown) => setError(errorLabel(reason)))
      .finally(() => setLoading(false));
  }, []);

  return (
    <main className="mx-auto min-h-screen max-w-5xl px-6 py-10">
      <Header />
      <section className="mt-8 rounded-2xl border border-[var(--line)] bg-white p-6 shadow-sm">
        <div className="mb-6 flex items-end justify-between gap-4">
          <div>
            <p className="text-sm font-semibold text-[var(--accent)]">Human review</p>
            <h1 className="mt-1 text-2xl font-semibold">Pending approvals</h1>
          </div>
          <span className="text-sm text-[var(--muted)]">No signing material is handled here.</span>
        </div>
        {loading && <Loading />}
        {!loading && error && <Failure message={error} />}
        {!loading && !error && approvals.filter((approval) => approval.status === "PENDING" && !isExpired(approval)).length === 0 && <EmptyState />}
        {!loading && !error && approvals.filter((approval) => approval.status === "PENDING" && !isExpired(approval)).length > 0 && (
          <div className="divide-y divide-[var(--line)]">
            {approvals.filter((approval) => approval.status === "PENDING" && !isExpired(approval)).map((approval) => (
              <Link className="grid gap-3 py-5 transition hover:bg-slate-50 sm:grid-cols-[1.4fr_1.4fr_0.8fr_1fr_1fr] sm:items-center" href={`/approvals/${encodeURIComponent(approval.approval_id)}`} key={approval.approval_id}>
                <div><p className="text-xs text-[var(--muted)]">Approval ID</p><p className="font-mono text-sm">{approval.approval_id}</p></div>
                <div><p className="text-xs text-[var(--muted)]">Intent ID</p><p className="font-mono text-sm">{approval.intent_id}</p></div>
                <div><p className="mb-1 text-xs text-[var(--muted)]">Status</p><Status approval={approval} /></div>
                <div><p className="text-xs text-[var(--muted)]">Created</p><p className="text-sm">{formatDate(approval.created_at)}</p></div>
                <div><p className="text-xs text-[var(--muted)]">Expires</p><p className="text-sm">{formatDate(approval.expires_at)}</p></div>
              </Link>
            ))}
          </div>
        )}
      </section>
    </main>
  );
}

export function ApprovalDetail({ approvalId }: { approvalId: string }) {
  const [approval, setApproval] = useState<Approval>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [actionError, setActionError] = useState<string>();
  const [submitting, setSubmitting] = useState<ApprovalDecision>();
  const [authorizing, setAuthorizing] = useState(false);
  const [authorizationId, setAuthorizationId] = useState<string>();

  useEffect(() => {
    getApproval(approvalId)
      .then(setApproval)
      .catch((reason: unknown) => setError(errorLabel(reason)))
      .finally(() => setLoading(false));
  }, [approvalId]);

  async function decide(decision: ApprovalDecision) {
    setSubmitting(decision);
    setActionError(undefined);
    try {
      const next = await decideApproval(approvalId, decision);
      setApproval((current) => current ? { ...current, ...next } : next);
    } catch (reason) {
      setActionError(errorLabel(reason));
    } finally {
      setSubmitting(undefined);
    }
  }

  async function confirmWalletExecution() {
    if (!approval) return;
    setAuthorizing(true);
    setActionError(undefined);
    try {
      const authorization = await authorizeExecution(approval);
      setAuthorizationId(authorization.execution_authorization_id);
      setApproval((current) => current ? { ...current, ...authorization } : current);
    } catch (reason) {
      setActionError(errorLabel(reason));
    } finally {
      setAuthorizing(false);
    }
  }

  return (
    <main className="mx-auto min-h-screen max-w-3xl px-6 py-10">
      <Header />
      <Link className="mt-8 inline-block text-sm font-semibold text-[var(--accent)] hover:underline" href="/approvals">← Back to approvals</Link>
      <section className="mt-4 rounded-2xl border border-[var(--line)] bg-white p-6 shadow-sm">
        {loading && <Loading />}
        {!loading && error && <Failure message={error} />}
        {!loading && !error && approval && (
          <>
            <div className="flex flex-wrap items-start justify-between gap-4 border-b border-[var(--line)] pb-6">
              <div><p className="text-sm font-semibold text-[var(--accent)]">Approval request</p><h1 className="mt-1 break-all font-mono text-xl font-semibold">{approval.approval_id}</h1></div>
              <Status approval={approval} />
            </div>
            <div className="grid gap-5 py-6 sm:grid-cols-2">
              <SafeField label="Intent" value={`${approval.intent_id} · version ${approval.intent_version}`} mono />
              <SafeField label="Intent type" value={approval.intent_type ?? "Not provided by API"} />
              <SafeField label="Wallet binding reference" value={approval.wallet_binding_reference ?? "Not provided by API"} mono />
              <SafeField label="Policy status" value={approval.policy_status ?? "Not provided by API"} />
              <SafeField label="Agent identity" value={approval.agent_identity ?? "Not provided by API"} />
              <SafeField label="Amount" value={approval.amount && approval.token ? `${approval.amount} ${approval.token}` : "Not provided by API"} />
              <SafeField label="Recipient" value={approval.recipient ?? "Not provided by API"} mono />
              <SafeField label="Wallet reference" value={approval.wallet_reference ?? "Not provided by API"} mono />
              <SafeField label="Expires" value={formatDate(approval.expires_at)} />
              <SafeField label="Created" value={formatDate(approval.created_at)} />
            </div>
            <div className="rounded-xl bg-slate-50 p-4 text-sm text-[var(--muted)]">Approve or reject only after reviewing the server-provided summary. This page never receives keys, tokens, provider payloads, or signing material.</div>
            {actionError && <Failure message={actionError} />}
            {authorizationId && <div className="mt-4 rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800">Wallet execution authorized for preparation. Authorization ID: <span className="font-mono">{authorizationId}</span>. No transaction was submitted.</div>}
            <div className="mt-6 flex flex-wrap gap-3">
              <button className="rounded-lg bg-emerald-600 px-5 py-2.5 text-sm font-semibold text-white transition hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-50" disabled={Boolean(submitting) || isExpired(approval) || approval.status !== "PENDING"} onClick={() => decide("APPROVED")} type="button">{submitting === "APPROVED" ? "Approving…" : "Approve"}</button>
              <button className="rounded-lg border border-red-200 px-5 py-2.5 text-sm font-semibold text-red-700 transition hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50" disabled={Boolean(submitting) || isExpired(approval) || approval.status !== "PENDING"} onClick={() => decide("REJECTED")} type="button">{submitting === "REJECTED" ? "Rejecting…" : "Reject"}</button>
            </div>
            {(approval.status === "APPROVED" || approval.status === "READY_FOR_EXECUTION_CONFIRMATION") && !isExpired(approval) && !authorizationId && <button className="mt-4 rounded-lg bg-[var(--accent)] px-5 py-2.5 text-sm font-semibold text-white transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50" disabled={authorizing} onClick={confirmWalletExecution} type="button">{authorizing ? "Preparing confirmation…" : "Confirm Wallet Execution"}</button>}
          </>
        )}
      </section>
    </main>
  );
}

function Header() {
  return <header className="flex items-center justify-between"><Link className="text-lg font-bold tracking-tight" href="/approvals">WizPay <span className="text-[var(--accent)]">Approvals</span></Link><span className="rounded-full border border-[var(--line)] bg-white px-3 py-1 text-xs font-medium text-[var(--muted)]">Human review boundary</span></header>;
}

function SafeField({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div><p className="text-xs font-medium uppercase tracking-wide text-[var(--muted)]">{label}</p><p className={`mt-1 break-all text-sm ${mono ? "font-mono" : ""}`}>{value}</p></div>;
}

function Loading() { return <div className="animate-pulse py-12 text-center text-sm text-[var(--muted)]">Loading approval data…</div>; }
function EmptyState() { return <div className="rounded-xl border border-dashed border-[var(--line)] py-12 text-center"><p className="font-semibold">No approvals configured</p><p className="mt-2 text-sm text-[var(--muted)]">Configure approval IDs for the current API foundation.</p></div>; }
function Failure({ message }: { message: string }) { return <div className="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-800">{message}</div>; }
