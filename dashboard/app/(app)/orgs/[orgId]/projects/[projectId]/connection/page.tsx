"use client";

import { useParams } from "next/navigation";
import { ConnectionPanel } from "@/components/projects/ConnectionPanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";

export default function ConnectionPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <div className="mx-auto max-w-6xl px-6 py-6">
      <ProjectGuard>
        <ConnectionPanel projectId={params.projectId} />
      </ProjectGuard>
    </div>
  );
}
