export type Provider = {
  request(args: { method: string; params?: readonly unknown[] }): Promise<unknown>;
  on(event: string, listener: (...args: unknown[]) => void): void;
  removeListener(event: string, listener: (...args: unknown[]) => void): void;
};
export type Wallet = { id: string; name: string; provider: Provider };
export function isProvider(value: unknown): value is Provider {
  if (!value || typeof value !== "object") return false;
  try { const p = value as Partial<Provider>; return typeof p.request === "function" && typeof p.on === "function" && typeof p.removeListener === "function"; } catch { return false; }
}
// Metadata is a display hint only. No wallet-supplied icons or URLs are consumed.
export function discover(target: EventTarget & { ethereum?: unknown }, update: (wallets: Wallet[]) => void): () => void {
  const wallets: Wallet[] = [];
  let legacy: unknown; try { legacy = target.ethereum; } catch { /* Unsupported injected getter. */ }
  const announce = (event: Event) => {
    try {
    const detail = (event as CustomEvent).detail;
    if (!detail || !isProvider(detail.provider) || !detail.info || typeof detail.info.uuid !== "string" ||
        !/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(detail.info.uuid) ||
        typeof detail.info.name !== "string" || !detail.info.name.trim() || detail.info.name.length > 100) return;
    if (wallets.some(w => w.id === detail.info.uuid || w.provider === detail.provider)) return;
    wallets.push({ id: detail.info.uuid, name: detail.info.name, provider: detail.provider });
    } catch { return; }
    update([...wallets]);
  };
  target.addEventListener("eip6963:announceProvider", announce);
  target.dispatchEvent(new Event("eip6963:requestProvider"));
  if (wallets.length === 0 && isProvider(legacy)) wallets.push({ id: "legacy", name: "Legacy injected wallet (identity unverified)", provider: legacy });
  update([...wallets]);
  return () => target.removeEventListener("eip6963:announceProvider", announce);
}
