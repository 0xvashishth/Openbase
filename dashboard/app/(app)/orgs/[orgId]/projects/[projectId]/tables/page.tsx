"use client";

import { useParams } from "next/navigation";
import { TableBrowser } from "@/components/data/TableBrowser";
import { ProjectGuard, ProjectHeader } from "@/components/projects/ProjectGuard";

export default function TablesPage() {
  const params = useParams<{ orgId: string; projectId: string }>();
  return (
    <div>
      <ProjectHeader title="Tables" subtitle="Browse collections and rows — like Supabase's table editor." />
      <div className="mx-auto max-w-6xl px-6 py-6">
        <ProjectGuard requireConnection toolName="Tables">
          <TableBrowser projectId={params.projectId} />
        </ProjectGuard>
      </div>
    </div>
  );
}
