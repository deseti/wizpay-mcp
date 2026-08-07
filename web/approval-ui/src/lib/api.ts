export type ApprovalStatus =
  | "PENDING"
  | "APPROVED"
  | "READY_FOR_EXECUTION_CONFIRMATION"
  | "REJECTED"
  | "EXPIRED"
  | "CONSUMED"
  | string;

export type ApprovalDecision = "APPROVED" | "REJECTED";

export type Approval = {
  approval_id: string;
  approval_version: number;
  intent_id: string;
  intent_version: number;
  status: ApprovalStatus;
  decision: string;
  created_at: string;
  expires_at: string;
  intent_type?: string;
  wallet_binding_reference?: string;
  policy_status?: string;
  agent_identity?: string;
  amount?: string;
  token?: string;
  recipient?: string;
  wallet_reference?: string;
  wallet_binding_version?: number;
};

export type ExecutionAuthorization = Approval & { execution_authorization_id: string; wallet_binding_reference: string };

export class ApprovalApiError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "ApprovalApiError";
    this.status = status;
    this.code = code;
  }
}

const apiBaseUrl = (process.env.NEXT_PUBLIC_APPROVAL_API_BASE_URL ?? "").replace(/\/$/, "");

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
    cache: "no-store",
  });

  if (!response.ok) {
    let message = "The approval request failed.";
    let code: string | undefined;
    try {
      const body = (await response.json()) as { error?: { message?: string; code?: string } };
      message = body.error?.message ?? message;
      code = body.error?.code;
    } catch {
      // Keep the safe generic message when the server did not return JSON.
    }
    throw new ApprovalApiError(message, response.status, code);
  }

  return (await response.json()) as T;
}

export function getApproval(approvalId: string): Promise<Approval> {
  return request<Approval>(`/approval/${encodeURIComponent(approvalId)}`);
}

export function decideApproval(approvalId: string, decision: ApprovalDecision): Promise<Approval> {
  return request<Approval>(`/approval/${encodeURIComponent(approvalId)}/decision`, {
    method: "POST",
    body: JSON.stringify({ decision }),
  });
}

export function authorizeExecution(approval: Approval): Promise<ExecutionAuthorization> {
  return request<ExecutionAuthorization>(`/approval/${encodeURIComponent(approval.approval_id)}/authorize-execution`, {
    method: "POST",
    body: JSON.stringify({ intent_id: approval.intent_id, wallet_binding_id: approval.wallet_binding_reference, wallet_binding_version: approval.wallet_binding_version }),
  });
}

export async function listConfiguredApprovals(): Promise<Approval[]> {
  const configuredIds = (process.env.NEXT_PUBLIC_APPROVAL_IDS ?? "")
    .split(",")
    .map((value) => value.trim())
    .filter(Boolean);

  if (configuredIds.length === 0) {
    return [];
  }
  return Promise.all(configuredIds.map((approvalId) => getApproval(approvalId)));
}
