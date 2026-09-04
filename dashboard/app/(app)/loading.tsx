import { CardGridSkeleton } from "@/components/ui/skeletons";

export default function AppLoading() {
  return (
    <div className="mx-auto max-w-6xl px-6 py-6">
      <CardGridSkeleton count={6} label="Loading page" />
    </div>
  );
}
