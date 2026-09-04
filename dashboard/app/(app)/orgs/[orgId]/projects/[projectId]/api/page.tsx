"use client";

import { useParams } from "next/navigation";
import { APIKeysPanel } from "@/components/projects/APIKeysPanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";

export default function ApiPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <div className="mx-auto max-w-6xl px-6 py-6">
      <ProjectGuard>
        <APIKeysPanel projectId={params.projectId} />
      </ProjectGuard>
    </div>
  );
}
