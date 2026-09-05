import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { OrgSettingsPanel } from "./OrgSettingsPanel";

const replace = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push: vi.fn() }),
}));

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
      updateOrg: vi.fn(),
      deleteOrg: vi.fn(),
      listProjects: vi.fn(),
      listMembers: vi.fn(),
    },
  };
});

vi.mock("@/components/AuthProvider", () => ({ authToken: () => "test-token" }));

const refresh = vi.fn();
let orgCtx: Record<string, unknown>;
vi.mock("@/lib/org-context", () => ({ useOrg: () => orgCtx }));

import { ApiError, api } from "@/lib/api";
import { can } from "@/lib/permissions";
import type { OrgRole } from "@/lib/types";
import { ToastProvider } from "@/components/ui/toast";

const ORG = {
  id: "o1",
  name: "Acme",
  slug: "acme",
  created_by: "u1",
  created_at: "2026-01-01T00:00:00Z",
};

function setRole(role: OrgRole) {
  orgCtx = {
    orgId: "o1",
    org: { ...ORG, role },
    role,
    settled: true,
    can: (action: string) => can(role, action),
    loading: false,
    error: null,
    refresh,
  };
}

function renderPanel() {
  return render(
    <ToastProvider>
      <OrgSettingsPanel />
    </ToastProvider>
  );
}

describe("OrgSettingsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listProjects).mockResolvedValue([] as never);
    vi.mocked(api.listMembers).mockResolvedValue([] as never);
    setRole("owner");
  });

  it("disables Save until something actually changes", async () => {
    const user = userEvent.setup();
    renderPanel();
    const save = screen.getByRole("button", { name: "Save changes" });
    expect(save).toBeDisabled();

    await user.type(screen.getByLabelText("Name"), " Corp");
    expect(save).toBeEnabled();
  });

  it("a member sees read-only inputs, a disabled Save, and the required role", () => {
    setRole("member");
    renderPanel();
    expect(screen.getByLabelText("Name")).toHaveAttribute("readonly");
    expect(screen.getByLabelText("Slug")).toHaveAttribute("readonly");
    expect(screen.getByRole("button", { name: "Save changes" })).toBeDisabled();
    expect(screen.getByText(/requires the admin or owner role/i)).toBeInTheDocument();
  });

  it("never renders the danger zone for a non-owner", () => {
    setRole("admin");
    renderPanel();
    expect(screen.queryByText("Danger zone")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete organization" })).not.toBeInTheDocument();
  });

  it("renders the danger zone for an owner", () => {
    renderPanel();
    expect(screen.getByText("Danger zone")).toBeInTheDocument();
  });

  /**
   * A slug collision is a field problem. Putting it in a page-level banner
   * makes the user hunt for which input to fix.
   */
  it("lands a 409 on the slug field, not a page banner", async () => {
    const user = userEvent.setup();
    vi.mocked(api.updateOrg).mockRejectedValue(new ApiError(409, "slug already in use"));
    renderPanel();

    const slug = screen.getByLabelText("Slug");
    await user.clear(slug);
    await user.type(slug, "taken");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/already in use/i));
    expect(slug).toHaveAttribute("aria-invalid", "true");
  });

  it("rejects a malformed slug before calling the API", async () => {
    const user = userEvent.setup();
    renderPanel();

    const slug = screen.getByLabelText("Slug");
    await user.clear(slug);
    await user.type(slug, "Not A Slug");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    expect(api.updateOrg).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("sends only the fields that changed", async () => {
    const user = userEvent.setup();
    vi.mocked(api.updateOrg).mockResolvedValue({ id: "o1", name: "Acme Corp", slug: "acme" } as never);
    renderPanel();

    await user.type(screen.getByLabelText("Name"), " Corp");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      // slug is unchanged, so it must be omitted rather than resent.
      expect(api.updateOrg).toHaveBeenCalledWith("test-token", "o1", "Acme Corp", undefined)
    );
    expect(refresh).toHaveBeenCalled();
  });

  it("requires typing the slug before deleting, then redirects", async () => {
    const user = userEvent.setup();
    vi.mocked(api.deleteOrg).mockResolvedValue({ deleted: true } as never);
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Delete organization" }));
    const dialog = screen.getByRole("dialog");
    const confirm = within(dialog).getByRole("button", { name: "Delete organization" });
    expect(confirm).toBeDisabled();

    await user.type(within(dialog).getByRole("textbox"), "acme");
    await user.click(confirm);

    await waitFor(() => expect(api.deleteOrg).toHaveBeenCalledWith("test-token", "o1", "acme"));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/orgs"));
  });

  /**
   * The server refuses to delete an org that still owns projects. "Conflict" is
   * a dead end; the blocking projects, each linked, are the actionable answer.
   */
  it("shows the blocking project list when delete returns 409", async () => {
    const user = userEvent.setup();
    vi.mocked(api.deleteOrg).mockRejectedValue(new ApiError(409, "organization has projects"));
    vi.mocked(api.listProjects).mockResolvedValue([
      { id: "p1", organization_id: "o1", name: "Web", slug: "web", created_by: "u", created_at: "" },
      { id: "p2", organization_id: "o1", name: "Mobile", slug: "mobile", created_by: "u", created_at: "" },
    ] as never);
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Delete organization" }));
    const dialog = screen.getByRole("dialog");
    await user.type(within(dialog).getByRole("textbox"), "acme");
    await user.click(within(dialog).getByRole("button", { name: "Delete organization" }));

    await waitFor(() =>
      expect(within(screen.getByRole("dialog")).getByText(/delete these projects first/i)).toBeInTheDocument()
    );
    const link = within(screen.getByRole("dialog")).getByRole("link", { name: "Web" });
    expect(link).toHaveAttribute("href", "/orgs/o1/projects/p1/settings");
    expect(replace).not.toHaveBeenCalled();
  });

  it("cancelling the dialog calls no API", async () => {
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByRole("button", { name: "Delete organization" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(api.deleteOrg).not.toHaveBeenCalled();
  });
});
