import { ProjectOverviewSkeleton } from "@/components/ui/skeletons";

export default function ProjectLoading() {
  return (
    <div className="mx-auto max-w-6xl px-6 py-6">
      <ProjectOverviewSkeleton />
    </div>
  );
}
