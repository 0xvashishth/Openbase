import { CardGridSkeleton } from "@/components/ui/skeletons";
import { PageShell } from "@/components/layout/PageShell";

export default function AppLoading() {
  return (
    <PageShell>
      <CardGridSkeleton count={6} label="Loading page" />
    </PageShell>
  );
}
