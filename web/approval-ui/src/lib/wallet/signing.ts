import type { Provider } from "./provider";
export function address(value: unknown): string {
  if (typeof value !== "string" || !/^0x[0-9a-fA-F]{40}$/.test(value)) throw new Error("invalid_account");
  return value;
}
export function accounts(value: unknown): string[] {
  if (!Array.isArray(value) || value.length === 0 || value.length > 100) throw new Error("no_accounts");
  return [...new Set(value.map(address))];
}
export function chain(value: unknown): boolean {
  return typeof value === "string" && value.length <= 18 && /^0x[0-9a-f]+$/i.test(value) && BigInt(value) === BigInt(5042);
}
export async function consistent(p: Provider, selected: string, current: () => void = () => {}): Promise<void> {
  const list = accounts(await p.request({ method: "eth_accounts" })); current();
  if (!list.some(a => a.toLowerCase() === selected.toLowerCase())) throw new Error("account_changed");
  const network = await p.request({ method: "eth_chainId" }); current();
  if (!chain(network)) throw new Error("wrong_network");
}
export function messageHex(message: string): string {
  return "0x" + Array.from(new TextEncoder().encode(message), byte => byte.toString(16).padStart(2, "0")).join("");
}
export async function sign(p: Provider, selected: string, message: string, current: () => void = () => {}): Promise<string> {
  await consistent(p, selected, current); current();
  const result = await p.request({ method: "personal_sign", params: [messageHex(message), selected] });
  if (typeof result !== "string" || !/^0x[0-9a-fA-F]{130}$/.test(result)) throw new Error("invalid_signature");
  await consistent(p, selected, current); current();
  return result;
}
