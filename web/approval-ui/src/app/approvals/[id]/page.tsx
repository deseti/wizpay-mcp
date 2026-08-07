import { ApprovalDetail } from "@/components/approval-ui";

export default async function ApprovalDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ApprovalDetail approvalId={decodeURIComponent(id)} />;
}
