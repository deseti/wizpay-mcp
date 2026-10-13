import { ApiError, TransportError } from "./onboarding-api";

// Only locally defined categories and messages reach the UI. Never echo provider
// rejection text, HTTP bodies, URLs, or authentication material.
const messages = {
  account_changed: "Wallet account changed. Restart authorization from your AI client.",
  chain_changed: "Wallet network changed. Select Arc Mainnet (5042), then restart from your AI client.",
  disconnected: "Wallet disconnected. Reconnect it, then restart from your AI client.",
  wrong_network: "Wrong network. Select Arc Mainnet (5042) manually; no network switch is requested.",
  session_mismatch: "Browser session does not match this authentication attempt. Restart from your AI client.",
  authorization_superseded: "Another OAuth authorization replaced this browser session. Continue in the newest authorization tab or restart from your AI client.",
  expired: "Authorization or authentication challenge expired or is no longer available. Restart from your AI client.",
  http_rejected: "The server rejected the authentication request. Restart from your AI client if the problem persists.",
  network_interrupted: "The connection was interrupted. Check connectivity and restart from your AI client.",
  timeout: "The authentication request timed out. Check connectivity and restart from your AI client.",
  verification_ambiguous: "The SIWE verification result could not be confirmed. No access was granted by this page. Restart from your AI client.",
  wallet_rejected: "The wallet request was rejected. You may try again.",
  enrollment_required: "Owner-approved enrollment is missing or inactive. Ask your authorized owner to stage this wallet and approve its application identity.",
  account_revoked: "This wallet account or tenant has been revoked. Contact the owner; revoked bindings cannot be reactivated.",
  unknown: "The authentication request could not be completed. Check your wallet and restart from your AI client.",
} as const;
export type Failure = keyof typeof messages;

export function failure(error: unknown): Failure {
  try {
    if (error instanceof TransportError && (error.kind === "timeout" || error.kind === "network_interrupted")) return error.kind;
    const value = error as { code?: unknown; message?: unknown } | null;
    if (value?.message === "enrollment_required" || value?.message === "account_revoked") return value.message;
    if (error instanceof ApiError) return error.status === 401 ? "expired" : "http_rejected";
    if (value?.code === 4001) return "wallet_rejected";
    switch (value?.message) {
      case "account_changed": case "wrong_network": case "session_mismatch": case "authorization_superseded": return value.message;
      case "expired_challenge": return "expired";
      case "invalid_session": case "invalid_transaction": case "invalid_evidence": case "missing_evidence": case "invalid_challenge": return "session_mismatch";
    }
  } catch { /* Wallet rejections may have hostile getters/prototypes. */ }
  return "unknown";
}

export function diagnostic(reason: Failure, ambiguous = false) {
  return {
    failure: reason,
    phase: reason === "wrong_network" ? "network" : reason === "wallet_rejected" ? "rejected" : reason === "expired" ? "expired" : "error",
    message: messages[reason] + (ambiguous && reason !== "verification_ambiguous" ? " " + messages.verification_ambiguous : ""),
  };
}
