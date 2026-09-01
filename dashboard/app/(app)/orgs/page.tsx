"use client";

import { useState } from "react";
import { OrgList } from "@/components/orgs/OrgList";
import { CreateOrgForm } from "@/components/orgs/CreateOrgForm";
import { Button } from "@/components/ui";

function PageHeader({ title, subtitle }: { title: string; subtitle: string }) {
  return (
    <header className="border-b border-slate-200 bg-white px-6 py-5">
      <h1 className="text-lg font-semibold text-slate-900">{title}</h1>
      <p className="mt-0.5 text-sm text-slate-500">{subtitle}</p>
    </header>
  );
}

export default function OrganizationsPage() {
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div>
      <PageHeader title="Organizations" subtitle="Everything in Openbase lives inside an organization." />
      <div className="mx-auto max-w-5xl px-6 py-6">
        <div className="mb-4 flex items-center justify-between">
          <p className="text-sm text-slate-500">Your organizations</p>
          <Button onClick={() => setShowCreate((v) => !v)} variant={showCreate ? "secondary" : "primary"}>
            {showCreate ? "Cancel" : "New organization"}
          </Button>
        </div>

        {showCreate && (
          <div className="mb-6 max-w-md rounded-xl border border-brand-200 bg-brand-50/40 p-5">
            <h2 className="mb-3 text-sm font-semibold text-slate-800">Create an organization</h2>
            <CreateOrgForm onCreated={() => setShowCreate(false)} />
          </div>
        )}

        <OrgList />
      </div>
    </div>
  );
}