"use client";

import { useParams } from "next/navigation";
import { DBSourcePanel } from "@/components/projects/DBSourcePanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";

export default function DBSourcePage() {
  const params = useParams<{ projectId: string }>();
  return (
    <div className="mx-auto max-w-6xl px-6 py-6">
      <ProjectGuard>
        <DBSourcePanel projectId={params.projectId} />
      </ProjectGuard>
    </div>
  );
}
