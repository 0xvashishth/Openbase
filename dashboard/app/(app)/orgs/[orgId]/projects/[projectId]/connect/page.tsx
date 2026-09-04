"use client";

import { useParams } from "next/navigation";
import { ConnectPanel } from "@/components/projects/ConnectPanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";

export default function ConnectPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <div className="mx-auto max-w-6xl px-6 py-6">
      <ProjectGuard>
        <ConnectPanel projectId={params.projectId} />
      </ProjectGuard>
    </div>
  );
}
