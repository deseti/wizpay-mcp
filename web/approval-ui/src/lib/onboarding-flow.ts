import { redirect, TransportError, type API, type Challenge, type View } from "./onboarding-api";
import { diagnostic, failure, type Failure } from "./onboarding-errors";
import { type Wallet } from "./wallet/provider";
import { accounts, chain, consistent, sign } from "./wallet/signing";
export type State = { phase: string; message: string; busy: boolean; restart: boolean; failure?: Failure; view?: View; wallet?: Wallet; accounts: string[]; account?: string; challenge?: Challenge };
export class Flow {
  state: State = { phase: "initializing", message: "Initializing authorization…", busy: false, restart: false, accounts: [] };
  private generation = 0;
  private changed = false;
  private verifying = false;
  private proof?: { challenge: string; pending: View };
  private sessionReads = 0;
  private observed?: View;
  private superseded = false;
  private reconciling?: Promise<void>;
  private disposed = false;
  private detach?: () => void;
  private providerSelection = 0;
  constructor(private api: API, private update: (s: State) => void, private navigate: (url: string) => void) {}
  private publish(p: Partial<State>) { if (this.disposed) return; this.state = { ...this.state, ...p }; this.update(this.state); }
  private current(g: number) { if (g !== this.generation || this.disposed) throw new Error("stale_flow"); }
  get canGrant() { return !this.disposed && !this.superseded && this.state.phase === "consent" && !this.state.busy && !this.state.restart && this.state.view?.state === "AUTHENTICATED" && this.state.view.authentication_available; }
  private sameAuthorization(v: View, expected: View) {
    return !!expected.transaction_id && v.transaction_id === expected.transaction_id && v.client_id === expected.client_id && v.resource === expected.resource && v.scope === expected.scope;
  }
  private authenticated(v: View, pending: View, challenge: string) {
    return v.state === "AUTHENTICATED" && this.sameAuthorization(v, pending) && v.csrf !== pending.csrf && v.challenge_id === challenge && !!v.user_id && v.authentication_available;
  }
  private checkAuthorization(v: View, expected: View) {
    if (v.transaction_id && v.transaction_id !== expected.transaction_id) {
      this.superseded = true;
      throw new Error("authorization_superseded");
    }
    if (!this.sameAuthorization(v, expected)) throw new Error("session_mismatch");
  }
  private reconcile(): Promise<void> {
    if (this.disposed || this.superseded) return Promise.resolve();
    if (this.reconciling) return this.reconciling;
    this.reconciling = this.cleanupAuthority().finally(() => { this.reconciling = undefined; });
    return this.reconciling;
  }
  private async cleanupAuthority() {
    const g = this.generation;
    const expected = this.proof?.pending ?? this.state.view;
    this.publish({ restart: true, challenge: undefined });
    try {
      if (!expected || (this.sessionReads >= 2 && !this.observed)) return;
      // Share the two-read budget with recovery. No third GET after exhaustion.
      let v = this.observed;
      if (!v) { this.sessionReads++; v = await this.api.session(); }
      this.current(g);
      this.checkAuthorization(v, expected);
      const owned = (v.state === "PENDING" && expected.state === "PENDING" && v.csrf === expected.csrf) ||
        (this.proof && this.authenticated(v, this.proof.pending, this.proof.challenge)) ||
        (expected.state === "AUTHENTICATED" && v.state === "AUTHENTICATED" && v.csrf === expected.csrf && v.challenge_id === expected.challenge_id && v.user_id === expected.user_id);
      // No await between the liveness/ownership checks and mutation initiation.
      if (owned && !this.superseded) await this.api.logout(v.csrf);
      this.current(g);
    } catch (e) {
      if (!this.disposed && g === this.generation && failure(e) === "authorization_superseded") this.publish(diagnostic("authorization_superseded"));
      // Failed cleanup never restores authority or repeats a mutation.
    } finally {
      if (!this.disposed && g === this.generation) this.publish({ view: undefined, challenge: undefined, restart: true, phase: "restart" });
    }
  }
  private async run(action: (g: number) => Promise<void>) {
    if (this.state.busy || this.state.restart || this.disposed) return;
    const g = this.generation;
    this.publish({ busy: true, failure: undefined });
    try { await action(g); } catch (e) {
      if (g === this.generation && !this.disposed) {
        const reason = failure(e);
        const ambiguous = this.verifying && ["unknown", "network_interrupted", "timeout"].includes(reason);
        const restart = this.verifying || this.superseded || ["expired", "session_mismatch", "account_changed"].includes(reason);
        this.publish({ ...diagnostic(ambiguous && reason === "unknown" ? "verification_ambiguous" : reason, ambiguous), challenge: undefined, restart, ...(restart ? { view: undefined } : {}) });
      }
    } finally {
      if (!this.disposed && !this.superseded && (this.changed || this.verifying)) await this.reconcile();
      this.changed = false; this.verifying = false;
      this.publish({ busy: false });
    }
  }
  initialize() { return this.run(async g => { const v = await this.api.session(); this.current(g); this.publish({ view: v, phase: v.state === "AUTHENTICATED" ? "consent" : v.state === "DENIED" ? "denied" : "selection", message: v.state === "AUTHENTICATED" ? "Authenticated. Review and separately authorize read-only access." : v.state === "DENIED" ? "Authorization denied. No MCP access was granted." : "Choose your wallet. Connecting alone does not authenticate you." }); }); }
  select(wallet: Wallet) {
    if (this.state.restart || this.disposed) return;
    if (this.state.busy || this.state.view?.state === "AUTHENTICATED") { this.invalidate("account_changed"); return; }
    this.cleanup(); this.generation++;
    this.proof = undefined; this.observed = undefined; this.sessionReads = 0;
    const selection = this.providerSelection;
    const listeners = ["accountsChanged", "chainChanged", "disconnect"].map(event => {
      const listener = () => {
        if (this.disposed || selection !== this.providerSelection) return;
        // Initial permission exposure establishes a candidate, not authenticated authority.
        // Returned accounts are subsequently checked against eth_accounts.
        if (event === "accountsChanged" && this.state.phase === "connecting" && !this.state.account && !this.state.challenge) return;
        this.invalidate(event === "chainChanged" ? "chain_changed" : event === "disconnect" ? "disconnected" : "account_changed");
      };
      return { event, listener };
    });
    this.detach = () => { for (const { event, listener } of listeners) { try { wallet.provider.removeListener(event, listener); } catch { /* Continue removing other listeners. */ } } };
    try { for (const { event, listener } of listeners) wallet.provider.on(event, listener); }
    catch { this.cleanup(); this.publish({ wallet: undefined, accounts: [], account: undefined, challenge: undefined, phase: "error", message: "This wallet does not support the required event interface." }); return; }
    this.publish({ wallet, account: undefined, accounts: [], challenge: undefined, phase: "connection", message: "Connect the selected wallet explicitly." });
  }
  private invalidate(reason: Failure) {
    if (this.disposed || this.state.restart) return;
    this.generation++; this.changed = true;
    this.publish({ ...diagnostic(reason), challenge: undefined, phase: "restart", restart: true });
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
    if (this.disposed || this.state.busy || this.state.restart || this.state.view?.state !== "PENDING" || !this.state.accounts.includes(account)) return;
    this.generation++; this.publish({ account, challenge: undefined });
  }
  requestChallenge() { return this.run(async g => {
    const { wallet, account, view } = this.state;
    if (!wallet || !account || view?.state !== "PENDING" || !view.authentication_available) throw new Error("unavailable");
    await consistent(wallet.provider, account, () => this.current(g)); this.current(g);
    this.publish({ phase: "challenge", message: "Requesting your authentication message…" });
    const c = await this.api.challenge(view.csrf, account); this.current(g);
    if (c.address.toLowerCase() !== account.toLowerCase()) throw new Error("invalid_challenge");
    if (Date.parse(c.expires_at) <= Date.now()) throw new Error("expired_challenge");
    this.publish({ challenge: c, phase: "message", message: "Review the exact message, then explicitly sign to authenticate." });
  }); }
  authenticate() { return this.run(async g => {
    const { wallet, account, view, challenge: c } = this.state;
    if (!wallet || !account || !c || view?.state !== "PENDING" || Date.parse(c.expires_at) <= Date.now()) throw new Error("expired_challenge");
    this.publish({ phase: "signing", message: "Awaiting your wallet’s authentication signature…" });
    const signature = await sign(wallet.provider, account, c.message, () => this.current(g)); this.current(g);
    if (Date.parse(c.expires_at) <= Date.now()) throw new Error("expired_challenge");
    if (!view.transaction_id) throw new Error("missing_transaction");
    this.proof = { challenge: c.challenge_id, pending: { ...view } };
    this.sessionReads = 0; this.observed = undefined;
    this.verifying = true;
    this.publish({ phase: "verifying", message: "Verifying wallet ownership…" });
    const result = await this.api.verify(view.csrf, c.challenge_id, signature); this.current(g);
    if (result.state === "ENROLLMENT_PENDING") {
      this.verifying = false;
      await this.api.logout(view.csrf); this.current(g);
      this.publish({ view: undefined, challenge: undefined, phase: "enrollment", restart: true, message: `Wallet control verified. Ask your authorized owner to approve enrollment using verification reference ${result.verification_id}. No access was granted. After approval, restart from Claude and sign a fresh authentication message.` });
      return;
    }
    this.publish({ phase: "refreshing", message: "Refreshing authenticated session authority…" });
    if (result.state !== "AUTHENTICATED") throw new Error("invalid_verification");
    let next: View;
    this.sessionReads++;
    try { next = await this.api.session(); }
    catch (e) {
      this.current(g);
      // Only an acknowledged verification plus a transport failure permits one
      // additional read. HTTP denials and invalid responses are not retried.
      if (!(e instanceof TransportError)) throw e;
      await consistent(wallet.provider, account, () => this.current(g)); this.current(g);
      this.sessionReads++;
      next = await this.api.session();
    }
    this.current(g); this.observed = next;
    this.checkAuthorization(next, view);
    if (!this.authenticated(next, view, c.challenge_id)) throw new Error("session_mismatch");
    await consistent(wallet.provider, account, () => this.current(g)); this.current(g);
    this.verifying = false;
    this.observed = undefined; this.sessionReads = 0;
    this.publish({ view: next, challenge: undefined, phase: "consent", message: "Authenticated. Granting read-only MCP access requires a separate decision." });
  }); }
  decide(decision: "grant" | "deny") { return this.run(async g => {
    const v = this.state.view;
    // run has marked busy; inspect authority directly rather than canGrant.
    if (!v || (decision === "grant" && (v.state !== "AUTHENTICATED" || !v.authentication_available))) throw new Error("unavailable");
    if (decision === "grant" && this.state.wallet && this.state.account) { await consistent(this.state.wallet.provider, this.state.account, () => this.current(g)); this.current(g); }
    try {
      const callback = await this.api.consent(v.csrf, decision); this.current(g);
      if (decision === "deny") this.publish({ view: { ...v, state: "DENIED" }, phase: "denied", restart: true, message: "Authorization denied. No MCP access was granted." });
      else { if (!callback) throw new Error("missing_callback"); this.publish({ phase: "returning", restart: true, message: "Returning to your AI client…" }); this.navigate(redirect(callback)); }
    } catch { if (!this.disposed && g === this.generation) this.publish({ view: undefined, restart: true, phase: "restart", message: "Consent completion is uncertain. Do not retry; restart from your AI client." }); }
  }); }
}
