import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ProjectSettingsPanel } from "./ProjectSettingsPanel";

const replace = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace, push: vi.fn() }) }));

vi.mock("@/lib/api", async () => {
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
      updateProject: vi.fn(),
      deleteProject: vi.fn(),
      listAPIKeys: vi.fn(),
      listTriggers: vi.fn(),
      listFunctions: vi.fn(),
    },
  };
});

vi.mock("@/components/AuthProvider", () => ({ authToken: () => "test-token" }));

let orgCtx: Record<string, unknown>;
let projectCtx: Record<string, unknown>;
vi.mock("@/lib/org-context", () => ({ useOrg: () => orgCtx }));
vi.mock("@/lib/project-context", () => ({ useProject: () => projectCtx }));

import { ApiError, api } from "@/lib/api";
import { can } from "@/lib/permissions";
import type { ConnectionMode, OrgRole } from "@/lib/types";
import { ToastProvider } from "@/components/ui/toast";

const PROJECT = {
  id: "p1",
  organization_id: "o1",
  name: "Web",
  slug: "web",
  created_by: "u1",
  created_at: "2026-01-01T00:00:00Z",
};

const refresh = vi.fn();

function setup({ role = "owner", mode }: { role?: OrgRole; mode?: ConnectionMode } = {}) {
  orgCtx = {
    orgId: "o1",
    org: { id: "o1", name: "Acme", slug: "acme", created_by: "u1", created_at: "", role },
    role,
    settled: true,
    can: (action: string) => can(role, action),
    loading: false,
    error: null,
    refresh: vi.fn(),
  };
  projectCtx = {
    orgId: "o1",
    projectId: "p1",
    project: PROJECT,
    connection: mode ? { id: "c1", project_id: "p1", mode, engine: "postgres", status: "connected" } : null,
    hasConnection: Boolean(mode),
    engine: mode ? "postgres" : null,
    capabilities: null,
    supportsTriggers: false,
    supportsRealtime: false,
    supportsForeignKeys: false,
    loading: false,
    error: null,
    refresh,
  };
}

function renderPanel() {
  return render(
    <ToastProvider>
      <ProjectSettingsPanel />
    </ToastProvider>
  );
}

describe("ProjectSettingsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listAPIKeys).mockResolvedValue([] as never);
    vi.mocked(api.listTriggers).mockResolvedValue([] as never);
    vi.mocked(api.listFunctions).mockResolvedValue([] as never);
    setup();
  });

  it("disables Save until a field changes", async () => {
    const user = userEvent.setup();
    renderPanel();
    const save = screen.getByRole("button", { name: "Save changes" });
    expect(save).toBeDisabled();
    await user.type(screen.getByLabelText("Name"), "site");
    expect(save).toBeEnabled();
  });

  it("a member sees read-only inputs and no delete control", () => {
    setup({ role: "member" });
    renderPanel();
    expect(screen.getByLabelText("Name")).toHaveAttribute("readonly");
    expect(screen.getByRole("button", { name: "Save changes" })).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Delete project" })).not.toBeInTheDocument();
  });

  /** Project deletion is owner-only; an admin must not see a button that 403s. */
  it("hides delete from an admin but keeps rename", () => {
    setup({ role: "admin" });
    renderPanel();
    expect(screen.getByLabelText("Name")).not.toHaveAttribute("readonly");
    expect(screen.queryByRole("button", { name: "Delete project" })).not.toBeInTheDocument();
  });

  it("puts a slug conflict on the slug field", async () => {
    const user = userEvent.setup();
    vi.mocked(api.updateProject).mockRejectedValue(new ApiError(409, "slug in use"));
    renderPanel();

    const slug = screen.getByLabelText("Slug");
    await user.clear(slug);
    await user.type(slug, "taken");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/already uses that slug/i));
    expect(slug).toHaveAttribute("aria-invalid", "true");
  });

  it("sends only changed fields", async () => {
    const user = userEvent.setup();
    vi.mocked(api.updateProject).mockResolvedValue(PROJECT as never);
    renderPanel();

    await user.type(screen.getByLabelText("Name"), "site");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(api.updateProject).toHaveBeenCalledWith("test-token", "p1", "Website", undefined)
    );
    expect(refresh).toHaveBeenCalled();
  });

  it("keeps confirm disabled until the slug matches exactly", async () => {
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByRole("button", { name: "Delete project" }));

    const dialog = screen.getByRole("dialog");
    const confirm = within(dialog).getByRole("button", { name: "Delete project" });
    expect(confirm).toBeDisabled();

    await user.type(within(dialog).getByRole("textbox"), "we");
    expect(confirm).toBeDisabled();

    await user.type(within(dialog).getByRole("textbox"), "b");
    expect(confirm).toBeEnabled();
  });

  it("cancel calls no API", async () => {
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByRole("button", { name: "Delete project" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(api.deleteProject).not.toHaveBeenCalled();
  });

  it("deletes then redirects to the org", async () => {
    const user = userEvent.setup();
    vi.mocked(api.deleteProject).mockResolvedValue({ deleted: true } as never);
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Delete project" }));
    const dialog = screen.getByRole("dialog");
    await user.type(within(dialog).getByRole("textbox"), "web");
    await user.click(within(dialog).getByRole("button", { name: "Delete project" }));

    await waitFor(() => expect(api.deleteProject).toHaveBeenCalledWith("test-token", "p1"));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/orgs/o1"));
  });

  /**
   * PHASES 18.1: provisioned databases have no persistent volumes, so deleting
   * the project really does destroy the data. The copy must not soften that.
   */
  it("warns that a provisioned database is unrecoverable", async () => {
    const user = userEvent.setup();
    setup({ mode: "provisioned" });
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Delete project" }));
    expect(
      within(screen.getByRole("dialog")).getByText(/not\s+recoverable — there is no backup/i)
    ).toBeInTheDocument();
  });

  it("tells a BYODB user their own database is left alone", async () => {
    const user = userEvent.setup();
    setup({ mode: "byodb" });
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Delete project" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/your database itself is left untouched/i)).toBeInTheDocument();
    expect(within(dialog).queryByText(/there is no backup/i)).not.toBeInTheDocument();
  });

  it("enumerates what will be destroyed", async () => {
    const user = userEvent.setup();
    vi.mocked(api.listAPIKeys).mockResolvedValue([
      { id: "k1", project_id: "p1", name: "prod", scopes: [], created_at: "" },
      { id: "k2", project_id: "p1", name: "ci", scopes: [], created_at: "" },
    ] as never);
    vi.mocked(api.listTriggers).mockResolvedValue([{ id: "t1" }] as never);
    renderPanel();
    await waitFor(() => expect(api.listAPIKeys).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: "Delete project" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/2 API keys/i)).toBeInTheDocument();
    expect(within(dialog).getByText(/1 trigger$/i)).toBeInTheDocument();
    expect(within(dialog).getByText(/0 functions/i)).toBeInTheDocument();
  });

  it("keeps the three quick links", () => {
    renderPanel();
    expect(screen.getByRole("link", { name: "Manage DB source" })).toHaveAttribute(
      "href",
      "/orgs/o1/projects/p1/db-source"
    );
    expect(screen.getByRole("link", { name: "Connect an app" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Manage API keys" })).toBeInTheDocument();
  });
});
