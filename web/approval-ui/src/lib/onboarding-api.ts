export type View = { transaction_id?: string; challenge_id?: string; state: "PENDING" | "AUTHENTICATED" | "DENIED"; client_id: string; client_name: string; resource: string; scope: string; user_id?: string; csrf: string; authentication_available: boolean };
export type Verification = { state: "AUTHENTICATED" | "ENROLLMENT_PENDING"; verification_id?: string };
export type Challenge = { purpose?: "AUTHENTICATE" | "ENROLL"; challenge_id: string; message: string; address: string; expires_at: string };
export class ApiError extends Error { constructor(public status: number, code?: string) { super(code === "enrollment_required" || code === "account_revoked" ? code : "The request could not be completed."); } }
export class TransportError extends Error {
  constructor(public kind: "network_interrupted" | "timeout") { super(kind); }
}
export function view(value: unknown): View {
  const v = value as View;
  if (!v || !["PENDING", "AUTHENTICATED", "DENIED"].includes(v.state) || !/^[A-Za-z0-9_-]{43}$/.test(v.csrf) ||
      (v.state !== "DENIED" && (typeof v.client_id !== "string" || typeof v.client_name !== "string" || v.resource !== "https://mcp.wizpay.xyz/mcp" || v.scope !== "mcp:read" || typeof v.authentication_available !== "boolean")) ||
      (v.state === "AUTHENTICATED" && (typeof v.user_id !== "string" || !v.user_id))) throw new Error("invalid_session");
  if (v.state !== "DENIED" && (typeof v.transaction_id !== "string" || !v.transaction_id || v.transaction_id.length > 256)) throw new Error("invalid_transaction");
  if (v.state === "AUTHENTICATED" && v.challenge_id === undefined) throw new Error("missing_evidence");
  if (v.challenge_id !== undefined && !/^[0-9a-f]{64}$/.test(v.challenge_id)) throw new Error("invalid_evidence");
  return v;
}
export function challenge(value: unknown): Challenge {
  const c = value as Challenge;
  if (!c || (c.purpose !== undefined && c.purpose !== "AUTHENTICATE" && c.purpose !== "ENROLL") || !/^[0-9a-f]{64}$/.test(c.challenge_id) || typeof c.message !== "string" || !c.message || new TextEncoder().encode(c.message).length > 2048 ||
      !/^0x[0-9a-fA-F]{40}$/.test(c.address) || typeof c.expires_at !== "string" || !Number.isFinite(Date.parse(c.expires_at))) throw new Error("invalid_challenge");
  return c;
}
export function redirect(value: unknown): string {
  if (typeof value !== "string" || value.length > 8192) throw new Error("invalid_redirect");
  const u = new URL(value);
  if (u.protocol !== "https:" || u.username || u.password || u.hash) throw new Error("invalid_redirect");
  return u.href;
}
export interface API { session(): Promise<View>; challenge(csrf: string, address: string): Promise<Challenge>; verify(csrf: string, id: string, signature: string): Promise<Verification>; consent(csrf: string, decision: "grant" | "deny"): Promise<string | void>; logout(csrf: string): Promise<void> }
export function browserAPI(): API {
  async function request(path: string, csrf?: string, body?: object): Promise<unknown> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 15000);
    try {
      let response: Response;
      try {
        response = await fetch(path, { method: csrf ? "POST" : "GET", credentials: "same-origin", redirect: "error", cache: "no-store", signal: controller.signal,
          headers: csrf ? { "X-CSRF-Token": csrf, ...(body ? { "Content-Type": "application/json" } : {}) } : {}, body: body ? JSON.stringify(body) : undefined });
      } catch { throw new TransportError(controller.signal.aborted ? "timeout" : "network_interrupted"); }
      if (!response.ok) { let code: string | undefined; try { const value = await response.json() as { error?: unknown }; if (typeof value?.error === "string") code = value.error; } catch {} throw new ApiError(response.status, code); }
      let text: string;
      try { text = await response.text(); }
      catch { throw new TransportError(controller.signal.aborted ? "timeout" : "network_interrupted"); }
      try { return JSON.parse(text); } catch { throw new Error("invalid_response"); }
    } finally { clearTimeout(timer); }
  }
  return {
    session: async () => view(await request("/browser/session")),
    challenge: async (csrf, address) => challenge(await request("/browser/siwe/challenge", csrf, { address })),
    verify: async (csrf, challenge_id, signature) => { const r = await request("/browser/siwe/verify", csrf, { challenge_id, signature }) as Verification; if (r?.state !== "AUTHENTICATED" && (r?.state !== "ENROLLMENT_PENDING" || r.verification_id !== challenge_id)) throw new Error("invalid_verification"); return r; },
    consent: async (csrf, decision) => { const r = await request("/browser/consent", csrf, { decision }) as { redirect?: unknown; state?: string }; if (decision === "grant") return redirect(r?.redirect); if (r?.state !== "complete") throw new Error("invalid_denial"); },
    logout: async csrf => { const r = await request("/browser/logout", csrf) as { state?: string }; if (r?.state !== "complete") throw new Error("invalid_logout"); },
  };
}
