import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { OrgProvider, useOrg } from "./org-context";

vi.mock("@/lib/api", async () => {
  class ApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  }
  return { ApiError, api: { getOrg: vi.fn() } };
});

vi.mock("@/components/AuthProvider", () => ({ authToken: () => "test-token" }));

import { ApiError, api } from "@/lib/api";

const ORG = {
  id: "o1",
  name: "Acme",
  slug: "acme",
  created_by: "u1",
  created_at: "2026-01-01T00:00:00Z",
};

function Probe() {
  const ctx = useOrg();
  return (
    <div>
      <p>loading:{String(ctx.loading)}</p>
      <p>settled:{String(ctx.settled)}</p>
      <p>role:{ctx.role}</p>
      <p>renameOrg:{String(ctx.can("rename_org"))}</p>
      <p>deleteOrg:{String(ctx.can("delete_org"))}</p>
      {ctx.error && <p role="alert">{ctx.error}</p>}
    </div>
  );
}

describe("OrgProvider", () => {
  beforeEach(() => vi.clearAllMocks());

  it("exposes the caller's role from the org payload", async () => {
    vi.mocked(api.getOrg).mockResolvedValue({ ...ORG, role: "owner" } as never);
    render(
      <OrgProvider orgId="o1">
        <Probe />
      </OrgProvider>
    );
    await waitFor(() => expect(screen.getByText("role:owner")).toBeInTheDocument());
    expect(screen.getByText("settled:true")).toBeInTheDocument();
    expect(screen.getByText("deleteOrg:true")).toBeInTheDocument();
  });

  it("gates admin out of owner-only actions", async () => {
    vi.mocked(api.getOrg).mockResolvedValue({ ...ORG, role: "admin" } as never);
    render(
      <OrgProvider orgId="o1">
        <Probe />
      </OrgProvider>
    );
    await waitFor(() => expect(screen.getByText("role:admin")).toBeInTheDocument());
    expect(screen.getByText("renameOrg:true")).toBeInTheDocument();
    expect(screen.getByText("deleteOrg:false")).toBeInTheDocument();
  });

  /**
   * The role arrives with the org, so there is a window where it is unknown.
   * Defaulting to "member" during that window is what stops a Delete button
   * from flashing enabled before the server's answer lands.
   */
  it("fails closed while the role is unknown", () => {
    vi.mocked(api.getOrg).mockReturnValue(new Promise(() => {}) as never);
    render(
      <OrgProvider orgId="o1">
        <Probe />
      </OrgProvider>
    );
    expect(screen.getByText("role:member")).toBeInTheDocument();
    expect(screen.getByText("renameOrg:false")).toBeInTheDocument();
    expect(screen.getByText("deleteOrg:false")).toBeInTheDocument();
    expect(screen.getByText("settled:false")).toBeInTheDocument();
  });

  it("treats a 403 as an access problem, not a transport failure", async () => {
    vi.mocked(api.getOrg).mockRejectedValue(new ApiError(403, "forbidden"));
    render(
      <OrgProvider orgId="o1">
        <Probe />
      </OrgProvider>
    );
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/do not have access/i));
  });

  it("keeps the server message for non-403 failures", async () => {
    vi.mocked(api.getOrg).mockRejectedValue(new ApiError(502, "metadata store unreachable"));
    render(
      <OrgProvider orgId="o1">
        <Probe />
      </OrgProvider>
    );
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("metadata store unreachable")
    );
  });

  /** An org with no role field must not silently grant privileges. */
  it("defaults to member when the payload omits role", async () => {
    vi.mocked(api.getOrg).mockResolvedValue(ORG as never);
    render(
      <OrgProvider orgId="o1">
        <Probe />
      </OrgProvider>
    );
    await waitFor(() => expect(screen.getByText("settled:true")).toBeInTheDocument());
    expect(screen.getByText("role:member")).toBeInTheDocument();
    expect(screen.getByText("renameOrg:false")).toBeInTheDocument();
  });
});
