"use client";

import { useState } from "react";
import { AuthUsersPanel } from "./AuthUsersPanel";
import { AuthProvidersPanel } from "./AuthProvidersPanel";
import { AuthHooksPanel } from "./AuthHooksPanel";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

/** Authentication section: end users, sign-in providers, auth hooks. */
export function AuthPanel({ projectId }: { projectId: string }) {
  const [tab, setTab] = useState("users");
  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-heading-sm font-medium tracking-tight">Authentication</h1>
        <p className="text-body-sm text-muted-foreground">
          Your app&apos;s end users, how they sign in, and what runs when they do.
        </p>
      </div>
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="users">Users</TabsTrigger>
          <TabsTrigger value="providers">Providers</TabsTrigger>
          <TabsTrigger value="hooks">Hooks</TabsTrigger>
        </TabsList>
        <TabsContent value="users">
          <AuthUsersPanel projectId={projectId} />
        </TabsContent>
        <TabsContent value="providers">
          <AuthProvidersPanel projectId={projectId} />
        </TabsContent>
        <TabsContent value="hooks">
          <AuthHooksPanel projectId={projectId} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
