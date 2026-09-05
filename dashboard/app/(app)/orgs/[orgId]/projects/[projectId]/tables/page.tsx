"use client";

import { useParams } from "next/navigation";
import { TableBrowser } from "@/components/data/TableBrowser";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { PageShell } from "@/components/layout/PageShell";

export default function TablesPage() {
  const params = useParams<{ orgId: string; projectId: string }>();
  return (
    <PageShell>
      <ProjectGuard requireConnection toolName="Tables">
        <TableBrowser projectId={params.projectId} />
      </ProjectGuard>
    </PageShell>
  );
}
