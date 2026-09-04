"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { OrgList } from "@/components/orgs/OrgList";
import { CreateOrgForm } from "@/components/orgs/CreateOrgForm";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";

export default function OrganizationsPage() {
  const [open, setOpen] = useState(false);

  return (
    <div className="mx-auto max-w-5xl px-6 py-6">
      <div className="mb-4 flex items-center justify-between">
        <p className="text-sm text-muted-foreground">Your organizations</p>
        <Button onClick={() => setOpen(true)}>
          <Plus className="h-4 w-4" aria-hidden /> New organization
        </Button>
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create an organization</DialogTitle>
            <DialogDescription>Organizations own projects and billing settings.</DialogDescription>
          </DialogHeader>
          <CreateOrgForm onCreated={() => setOpen(false)} />
        </DialogContent>
      </Dialog>

      <OrgList />
    </div>
  );
}
