import { redirect } from "next/navigation";

export default async function ApiKeysAlias({ params }: { params: Promise<{ orgId: string; projectId: string }> }) {
  const { orgId, projectId } = await params;
  redirect(`/orgs/${orgId}/projects/${projectId}/api`);
}
