import { redirect } from "next/navigation";

/** Legacy alias: the Connection tab was renamed to DB Source. */
export default async function ConnectionAlias({
  params,
}: {
  params: Promise<{ orgId: string; projectId: string }>;
}) {
  const { orgId, projectId } = await params;
  redirect(`/orgs/${orgId}/projects/${projectId}/db-source`);
}
