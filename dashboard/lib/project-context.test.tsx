import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ProjectProvider, useProject } from "./project-context";

vi.mock("@/lib/api", async () => {
  // ApiError is a real class used for status-aware branching, so keep it.
  class ApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  }
  return {
    ApiError,
    api: {
      getProject: vi.fn(),
      listProjects: vi.fn(),
      getConnection: vi.fn(),
      getFullSchema: vi.fn(),
    },
  };
});

vi.mock("@/components/AuthProvider", () => ({
  authToken: () => "test-token",
}));

import { ApiError, api } from "@/lib/api";

const PROJECT = {
  id: "p1",
  organization_id: "o1",
  name: "Shop",
  slug: "shop",
  created_by: "u",
  created_at: "",
};

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
    vi.mocked(api.getProject).mockResolvedValue(PROJECT as never);
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

  /**
   * Resolving one project must cost one request. Listing every project in the
   * org to find an id we already have is what getProject replaced.
   */
  it("resolves the project directly instead of listing the org", async () => {
    vi.mocked(api.getProject).mockResolvedValue(PROJECT as never);
    vi.mocked(api.getConnection).mockRejectedValue(new ApiError(404, "none"));

    render(
      <ProjectProvider orgId="o1" projectId="p1">
        <Probe />
      </ProjectProvider>
    );
    await waitFor(() => expect(screen.getByText("project:Shop")).toBeInTheDocument());
    expect(api.getProject).toHaveBeenCalledWith("test-token", "p1");
    expect(api.listProjects).not.toHaveBeenCalled();
  });

  it("handles missing connection gracefully (not connected, caps default off)", async () => {
    vi.mocked(api.getProject).mockResolvedValue(PROJECT as never);
    vi.mocked(api.getConnection).mockRejectedValue(new ApiError(404, "nope"));

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
    vi.mocked(api.getProject).mockRejectedValue(new ApiError(404, "not found"));
    render(
      <ProjectProvider orgId="o1" projectId="missing">
        <Probe />
      </ProjectProvider>
    );
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/not found/i));
  });

  /**
   * A transport failure is not a missing project: it must keep the server's
   * message rather than telling the user their project does not exist.
   */
  it("distinguishes a server error from a missing project", async () => {
    vi.mocked(api.getProject).mockRejectedValue(new ApiError(502, "database unreachable"));
    render(
      <ProjectProvider orgId="o1" projectId="p1">
        <Probe />
      </ProjectProvider>
    );
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("database unreachable"));
  });
});
