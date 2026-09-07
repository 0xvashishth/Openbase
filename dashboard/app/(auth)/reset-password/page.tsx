import { Suspense } from "react";
import { ResetPasswordForm } from "@/components/auth/ResetPasswordForm";

export default function ResetPasswordPage() {
  return (
    <div className="flex min-h-full items-center justify-center bg-background px-4 py-16">
      {/* useSearchParams() inside ResetPasswordForm requires a Suspense
          boundary so Next.js can prerender the page shell. */}
      <Suspense
        fallback={
          <div className="w-full max-w-sm">
            <p className="text-body-sm text-muted-foreground">Loading…</p>
          </div>
        }
      >
        <ResetPasswordForm />
      </Suspense>
    </div>
  );
}
