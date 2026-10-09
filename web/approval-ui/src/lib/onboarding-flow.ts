import { redirect, ApiError, type API, type Challenge, type View } from "./onboarding-api";
import { type Wallet } from "./wallet/provider";
import { accounts, chain, consistent, sign } from "./wallet/signing";
export type State = { phase: string; message: string; busy: boolean; restart: boolean; view?: View; wallet?: Wallet; accounts: string[]; account?: string; challenge?: Challenge };
export class Flow {
  state: State = { phase: "initializing", message: "Initializing authorization…", busy: false, restart: false, accounts: [] };
  private generation = 0;
  private changed = false;
  private verifying = false;
  private proof?: { challenge: string; transaction: string };
  private disposed = false;
  private detach?: () => void;
  private providerSelection = 0;
  constructor(private api: API, private update: (s: State) => void, private navigate: (url: string) => void) {}
  private publish(p: Partial<State>) { this.state = { ...this.state, ...p }; if (!this.disposed) this.update(this.state); }
  private current(g: number) { if (g !== this.generation || this.disposed) throw new Error("stale_flow"); }
  get canGrant() { return this.state.phase === "consent" && !this.state.busy && !this.state.restart && this.state.view?.state === "AUTHENTICATED" && this.state.view.authentication_available; }
  private async reconcile() {
    // Aborts/failed responses do not prove rollback. Never reuse pending CSRF.
    try { const v = await this.api.session(); if ((v.state === "AUTHENTICATED" && v.challenge_id === this.proof?.challenge && v.transaction_id === this.proof?.transaction) || (v.state === "PENDING" && v.csrf === this.state.view?.csrf && v.transaction_id === this.state.view?.transaction_id)) await this.api.logout(v.csrf); } catch { /* Remain fail-closed if authority cannot be reconciled. */ }
    this.publish({ view: undefined, challenge: undefined, restart: true, phase: "restart", message: "Wallet or session authority changed. Restart from your AI client; session cleanup may require retry." });
  }
  private async run(action: (g: number) => Promise<void>) {
    if (this.state.busy || this.state.restart || this.disposed) return;
    const g = this.generation;
    this.publish({ busy: true });
    try { await action(g); } catch (e) {
      if (g === this.generation && !this.disposed) {
        let error: { code?: unknown; message?: unknown } = {};
        try { if (e && typeof e === "object") { const candidate = e as { code?: unknown; message?: unknown }; error = { code: candidate.code, message: candidate.message }; } } catch { /* Untrusted rejection getters. */ }
        let expired = false;
        try { expired = e instanceof ApiError && e.status === 401; } catch { /* Malformed rejection prototype. */ }
        const code = error.code;
        this.publish({ challenge: undefined, phase: expired ? "expired" : error.message === "wrong_network" ? "network" : code === 4001 ? "rejected" : "error",
          message: error.message === "wrong_network" ? "Wrong network. Select Arc Mainnet (5042) manually; no network switch is requested." : code === 4001 ? "The wallet request was rejected. You may try again." : "The wallet or connection could not be verified. Check your account and network, or restart from your AI client." });
      }
    } finally {
      if (this.changed || this.verifying) { this.changed = false; this.verifying = false; await this.reconcile(); }
      this.publish({ busy: false });
    }
  }
  initialize() { return this.run(async g => { const v = await this.api.session(); this.current(g); this.publish({ view: v, phase: v.state === "AUTHENTICATED" ? "consent" : v.state === "DENIED" ? "denied" : "selection", message: v.state === "AUTHENTICATED" ? "Authenticated. Review and separately authorize read-only access." : v.state === "DENIED" ? "Authorization denied. No MCP access was granted." : "Choose your wallet. Connecting alone does not authenticate you." }); }); }
  select(wallet: Wallet) {
    if (this.state.restart || this.disposed) return;
    if (this.state.busy || this.state.view?.state === "AUTHENTICATED") { this.invalidate(); return; }
    this.cleanup(); this.generation++;
    const selection = this.providerSelection;
    const listeners = ["accountsChanged", "chainChanged", "disconnect"].map(event => {
      const listener = () => {
        if (this.disposed || selection !== this.providerSelection) return;
        // Initial permission exposure establishes a candidate, not authenticated authority.
        // Returned accounts are subsequently checked against eth_accounts.
        if (event === "accountsChanged" && this.state.phase === "connecting" && !this.state.account && !this.state.challenge) return;
        this.invalidate();
      };
      return { event, listener };
    });
    this.detach = () => { for (const { event, listener } of listeners) { try { wallet.provider.removeListener(event, listener); } catch { /* Continue removing other listeners. */ } } };
    try { for (const { event, listener } of listeners) wallet.provider.on(event, listener); }
    catch { this.cleanup(); this.publish({ wallet: undefined, accounts: [], account: undefined, challenge: undefined, phase: "error", message: "This wallet does not support the required event interface." }); return; }
    this.publish({ wallet, account: undefined, accounts: [], challenge: undefined, phase: "connection", message: "Connect the selected wallet explicitly." });
  }
  private invalidate() {
    this.generation++; this.changed = true;
    this.publish({ challenge: undefined, phase: "restart", restart: true, message: "Wallet changed. Consent is suspended; restart authorization." });
    if (!this.state.busy) void this.reconcile().finally(() => { this.changed = false; });
  }
  private cleanup() { this.providerSelection++; try { this.detach?.(); } catch { /* Provider cleanup cannot restore authority. */ } this.detach = undefined; }
  dispose() { this.disposed = true; this.generation++; this.cleanup(); }
  connect() { return this.run(async g => {
    const p = this.state.wallet?.provider; if (!p) throw new Error("missing_wallet");
    this.publish({ phase: "connecting", message: "Awaiting wallet account permission…" });
    const list = accounts(await p.request({ method: "eth_requestAccounts" })); this.current(g);
    this.publish({ accounts: list, account: list.length === 1 ? list[0] : undefined });
    const exposed = accounts(await p.request({ method: "eth_accounts" })); this.current(g);
    if (list.some(a => !exposed.some(b => a.toLowerCase() === b.toLowerCase()))) throw new Error("account_changed");
    const valid = chain(await p.request({ method: "eth_chainId" })); this.current(g);
    this.publish({ phase: valid ? "connected" : "network", message: valid ? "Connected, not authenticated. Select an account and request a message." : "Wrong network. Select Arc Mainnet (5042) manually in your wallet, then restart authorization if a change event occurs." });
  }); }
  chooseAccount(account: string) {
    if (this.state.busy || this.state.restart || this.state.view?.state !== "PENDING" || !this.state.accounts.includes(account)) return;
    this.generation++; this.publish({ account, challenge: undefined });
  }
  requestChallenge() { return this.run(async g => {
    const { wallet, account, view } = this.state;
    if (!wallet || !account || view?.state !== "PENDING" || !view.authentication_available) throw new Error("unavailable");
    await consistent(wallet.provider, account); this.current(g);
    this.publish({ phase: "challenge", message: "Requesting your authentication message…" });
    const c = await this.api.challenge(view.csrf, account); this.current(g);
    if (c.address.toLowerCase() !== account.toLowerCase() || Date.parse(c.expires_at) <= Date.now()) throw new Error("invalid_challenge");
    this.publish({ challenge: c, phase: "message", message: "Review the exact message, then explicitly sign to authenticate." });
  }); }
  authenticate() { return this.run(async g => {
    const { wallet, account, view, challenge: c } = this.state;
    if (!wallet || !account || !c || view?.state !== "PENDING" || Date.parse(c.expires_at) <= Date.now()) throw new Error("expired_challenge");
    this.publish({ phase: "signing", message: "Awaiting your wallet’s authentication signature…" });
    const signature = await sign(wallet.provider, account, c.message, () => this.current(g)); this.current(g);
    if (Date.parse(c.expires_at) <= Date.now()) throw new Error("expired_challenge");
    if (!view.transaction_id) throw new Error("missing_transaction");
    this.proof = { challenge: c.challenge_id, transaction: view.transaction_id };
    this.verifying = true;
    this.publish({ phase: "verifying", message: "Verifying wallet ownership…" });
    await this.api.verify(view.csrf, c.challenge_id, signature); this.current(g);
    this.publish({ phase: "refreshing", message: "Refreshing authenticated session authority…" });
    const next = await this.api.session(); this.current(g);
    if (next.state !== "AUTHENTICATED" || next.client_id !== view.client_id || next.resource !== view.resource || next.scope !== view.scope || next.csrf === view.csrf || next.challenge_id !== c.challenge_id || next.transaction_id !== view.transaction_id) throw new Error("session_mismatch");
    this.verifying = false;
    this.publish({ view: next, challenge: undefined, phase: "consent", message: "Authenticated. Granting read-only MCP access requires a separate decision." });
  }); }
  decide(decision: "grant" | "deny") { return this.run(async g => {
    const v = this.state.view;
    // run has marked busy; inspect authority directly rather than canGrant.
    if (!v || (decision === "grant" && (v.state !== "AUTHENTICATED" || !v.authentication_available))) throw new Error("unavailable");
    if (decision === "grant" && this.state.wallet && this.state.account) { await consistent(this.state.wallet.provider, this.state.account); this.current(g); }
    try {
      const callback = await this.api.consent(v.csrf, decision); this.current(g);
      if (decision === "deny") this.publish({ view: { ...v, state: "DENIED" }, phase: "denied", restart: true, message: "Authorization denied. No MCP access was granted." });
      else { if (!callback) throw new Error("missing_callback"); this.publish({ phase: "returning", restart: true, message: "Returning to your AI client…" }); this.navigate(redirect(callback)); }
    } catch { this.publish({ view: undefined, restart: true, phase: "restart", message: "Consent completion is uncertain. Do not retry; restart from your AI client." }); }
  }); }
}
