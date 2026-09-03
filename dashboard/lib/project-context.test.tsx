import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ProjectProvider, useProject } from "./project-context";

vi.mock("@/lib/api", () => ({
  api: {
    listProjects: vi.fn(),
    getConnection: vi.fn(),
    getFullSchema: vi.fn(),
  },
}));

vi.mock("@/components/AuthProvider", () => ({
  authToken: () => "test-token",
}));

import { api } from "@/lib/api";

function Probe() {
  const ctx = useProject();
  if (ctx.loading) return <p>loading</p>;
  if (ctx.error) return <p role="alert">{ctx.error}</p>;
  return (
    <div>
      <p>project:{ctx.project?.name}</p>
      <p>connected:{String(ctx.hasConnection)}</p>
      <p>triggers:{String(ctx.supportsTriggers)}</p>
      <p>realtime:{String(ctx.supportsRealtime)}</p>
    </div>
  );
}

describe("ProjectProvider", () => {
  beforeEach(() => vi.clearAllMocks());

  it("loads project + connection + capabilities", async () => {
    vi.mocked(api.listProjects).mockResolvedValue([
      { id: "p1", organization_id: "o1", name: "Shop", slug: "shop", created_by: "u", created_at: "" },
    ] as never);
    vi.mocked(api.getConnection).mockResolvedValue({
      status: "connected",
      engine: "postgres",
    } as never);
    vi.mocked(api.getFullSchema).mockResolvedValue({
      capabilities: {
        supports_native_triggers: true,
        supports_realtime: "native",
        supports_foreign_keys: true,
      },
    } as never);

    render(
      <ProjectProvider orgId="o1" projectId="p1">
        <Probe />
      </ProjectProvider>
    );
    await waitFor(() => expect(screen.getByText("project:Shop")).toBeInTheDocument());
    expect(screen.getByText("connected:true")).toBeInTheDocument();
    expect(screen.getByText("triggers:true")).toBeInTheDocument();
    expect(screen.getByText("realtime:true")).toBeInTheDocument();
  });

  it("handles missing connection gracefully (not connected, caps default off)", async () => {
    vi.mocked(api.listProjects).mockResolvedValue([
      { id: "p1", organization_id: "o1", name: "Shop", slug: "shop", created_by: "u", created_at: "" },
    ] as never);
    vi.mocked(api.getConnection).mockRejectedValue(Object.assign(new Error("nope"), { status: 404 }));

    render(
      <ProjectProvider orgId="o1" projectId="p1">
        <Probe />
      </ProjectProvider>
    );
    await waitFor(() => expect(screen.getByText("project:Shop")).toBeInTheDocument());
    expect(screen.getByText("connected:false")).toBeInTheDocument();
    expect(screen.getByText("triggers:false")).toBeInTheDocument();
  });

  it("surfaces project-not-found error", async () => {
    vi.mocked(api.listProjects).mockResolvedValue([] as never);
    render(
      <ProjectProvider orgId="o1" projectId="missing">
        <Probe />
      </ProjectProvider>
    );
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/not found/i));
  });
});
