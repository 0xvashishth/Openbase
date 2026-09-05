import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MembersPanel } from "./MembersPanel";

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
      me: vi.fn(),
      listMembers: vi.fn(),
      addMember: vi.fn(),
      updateMemberRole: vi.fn(),
      removeMember: vi.fn(),
      transferOwnership: vi.fn(),
    },
  };
});

vi.mock("@/components/AuthProvider", () => ({ authToken: () => "test-token" }));

let orgCtx: Record<string, unknown>;
vi.mock("@/lib/org-context", () => ({ useOrg: () => orgCtx }));

import { ApiError, api } from "@/lib/api";
import { can } from "@/lib/permissions";
import type { OrgMember, OrgRole } from "@/lib/types";
import { ToastProvider } from "@/components/ui/toast";

const OWNER: OrgMember = {
  user_id: "u1",
  email: "owner@example.com",
  full_name: "Olive Owner",
  role: "owner",
  joined_at: "2026-01-01T00:00:00Z",
};
const ADMIN: OrgMember = {
  user_id: "u2",
  email: "admin@example.com",
  full_name: "Adam Admin",
  role: "admin",
  joined_at: "2026-02-01T00:00:00Z",
};
const MEMBER: OrgMember = {
  user_id: "u3",
  email: "member@example.com",
  role: "member",
  joined_at: "2026-03-01T00:00:00Z",
};

function setViewer(role: OrgRole) {
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
}

function renderPanel() {
  return render(
    <ToastProvider>
      <MembersPanel />
    </ToastProvider>
  );
}

/**
 * A member with no full_name renders their email twice (as the display name and
 * as the muted secondary line), so match on the first occurrence — both live in
 * the same row.
 */
function rowFor(email: string) {
  return screen.getAllByText(email)[0].closest("tr") as HTMLElement;
}

describe("MembersPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listMembers).mockResolvedValue([OWNER, ADMIN, MEMBER] as never);
    vi.mocked(api.me).mockResolvedValue({ id: "u1", email: OWNER.email } as never);
    setViewer("owner");
  });

  it("renders names and emails, and marks your own row", async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());
    expect(screen.getByText("admin@example.com")).toBeInTheDocument();
    expect(screen.getByText("Adam Admin")).toBeInTheDocument();
    // No full_name — fall back to the email as the display name.
    expect(screen.getAllByText("member@example.com").length).toBeGreaterThan(0);
    expect(within(rowFor("owner@example.com")).getByText("You")).toBeInTheDocument();
  });

  it("shows the member count", async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText("3 members")).toBeInTheDocument());
  });

  /**
   * The last owner's role select must be disabled with a reason: demoting them
   * would leave the org ownerless, which the server rejects with 409.
   */
  it("disables the last owner's role select and states why", async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());
    const select = within(rowFor("owner@example.com")).getByLabelText("Role");
    expect(select).toBeDisabled();
    expect(select.parentElement).toHaveAttribute(
      "title",
      "An organization must keep at least one owner."
    );
  });

  it("enables role selects once a second owner exists", async () => {
    vi.mocked(api.listMembers).mockResolvedValue([OWNER, { ...ADMIN, role: "owner" }] as never);
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());
    expect(within(rowFor("owner@example.com")).getByLabelText("Role")).toBeEnabled();
  });

  it("offers Leave on your own row rather than Remove", async () => {
    const user = userEvent.setup();
    vi.mocked(api.listMembers).mockResolvedValue([OWNER, { ...ADMIN, role: "owner" }] as never);
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());

    await user.click(
      within(rowFor("owner@example.com")).getByRole("button", { name: /actions for owner@example.com/i })
    );
    expect(await screen.findByText("Leave organization")).toBeInTheDocument();
  });

  it("blocks the menu for a sole owner viewing their own row", async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());
    const blocked = within(rowFor("owner@example.com")).getByLabelText("Transfer ownership before leaving.");
    expect(blocked).toHaveAttribute("aria-disabled", "true");
  });

  /** An admin has no authority over an owner row; the server would 403. */
  it("offers an admin no actions on an owner row", async () => {
    setViewer("admin");
    vi.mocked(api.me).mockResolvedValue({ id: "u2", email: ADMIN.email } as never);
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());

    const ownerRow = rowFor("owner@example.com");
    expect(within(ownerRow).queryByLabelText("Role")).not.toBeInTheDocument();
    expect(within(ownerRow).getByLabelText("Only owners can manage owners.")).toBeInTheDocument();
  });

  it("never offers the owner role to an admin in the add dialog", async () => {
    const user = userEvent.setup();
    setViewer("admin");
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "Add member" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("button", { name: "Member" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Admin" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Owner" })).not.toBeInTheDocument();
  });

  it("offers the owner role to an owner", async () => {
    const user = userEvent.setup();
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: "Add member" }));
    expect(within(screen.getByRole("dialog")).getByRole("button", { name: "Owner" })).toBeInTheDocument();
  });

  it("a member sees a read-only table and an explanation", async () => {
    setViewer("member");
    vi.mocked(api.me).mockResolvedValue({ id: "u3", email: MEMBER.email } as never);
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());

    expect(screen.queryByRole("button", { name: "Add member" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Role")).not.toBeInTheDocument();
    expect(screen.getByText(/only owners and admins can manage members/i)).toBeInTheDocument();
  });

  /**
   * With no mailer (PHASES 9.2) an unknown email cannot be invited. The copy has
   * to say that rather than implying an invite was sent.
   */
  it("explains that an unknown email must sign up first", async () => {
    const user = userEvent.setup();
    vi.mocked(api.addMember).mockRejectedValue(new ApiError(404, "no account"));
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "Add member" }));
    const dialog = screen.getByRole("dialog");
    await user.type(within(dialog).getByLabelText("Email"), "nobody@example.com");
    await user.click(within(dialog).getByRole("button", { name: "Add member" }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(/they need to sign up first/i)
    );
  });

  it("reports a duplicate member as a conflict, not a failure", async () => {
    const user = userEvent.setup();
    vi.mocked(api.addMember).mockRejectedValue(new ApiError(409, "already a member"));
    renderPanel();
    await waitFor(() => expect(screen.getByText("owner@example.com")).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "Add member" }));
    const dialog = screen.getByRole("dialog");
    await user.type(within(dialog).getByLabelText("Email"), "admin@example.com");
    await user.click(within(dialog).getByRole("button", { name: "Add member" }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(/already a member of this organization/i)
    );
  });

  it("promotes a member to admin", async () => {
    const user = userEvent.setup();
    vi.mocked(api.updateMemberRole).mockResolvedValue({ role: "admin" } as never);
    renderPanel();
    await waitFor(() => expect(screen.getAllByText("member@example.com").length).toBeGreaterThan(0));

    await user.selectOptions(within(rowFor("member@example.com")).getByLabelText("Role"), "admin");
    await waitFor(() =>
      expect(api.updateMemberRole).toHaveBeenCalledWith("test-token", "o1", "u3", "admin")
    );
  });

  /** Optimistic update must roll back when the server refuses. */
  it("rolls back an optimistic role change on failure", async () => {
    const user = userEvent.setup();
    vi.mocked(api.updateMemberRole).mockRejectedValue(new ApiError(409, "at least one owner"));
    renderPanel();
    await waitFor(() => expect(screen.getAllByText("member@example.com").length).toBeGreaterThan(0));

    await user.selectOptions(within(rowFor("member@example.com")).getByLabelText("Role"), "admin");
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/could not change role/i));
    expect(within(rowFor("member@example.com")).getByLabelText("Role")).toHaveValue("member");
  });

  it("transfer ownership defaults to demoting yourself but allows opting out", async () => {
    const user = userEvent.setup();
    vi.mocked(api.transferOwnership).mockResolvedValue({} as never);
    renderPanel();
    await waitFor(() => expect(screen.getByText("admin@example.com")).toBeInTheDocument());

    await user.click(
      within(rowFor("admin@example.com")).getByRole("button", { name: /actions for admin@example.com/i })
    );
    await user.click(await screen.findByText("Transfer ownership"));

    const dialog = screen.getByRole("dialog");
    const checkbox = within(dialog).getByRole("checkbox");
    expect(checkbox).toBeChecked();
    await user.click(checkbox);
    await user.click(within(dialog).getByRole("button", { name: "Transfer ownership" }));

    await waitFor(() =>
      expect(api.transferOwnership).toHaveBeenCalledWith("test-token", "o1", "u2", false)
    );
  });

  it("removes another member after confirmation", async () => {
    const user = userEvent.setup();
    vi.mocked(api.removeMember).mockResolvedValue({ left: false } as never);
    renderPanel();
    await waitFor(() => expect(screen.getAllByText("member@example.com").length).toBeGreaterThan(0));

    await user.click(
      within(rowFor("member@example.com")).getByRole("button", { name: /actions for member@example.com/i })
    );
    await user.click(await screen.findByText("Remove from organization"));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Remove member" }));

    await waitFor(() => expect(api.removeMember).toHaveBeenCalledWith("test-token", "o1", "u3"));
  });

  it("surfaces a load failure instead of an empty table", async () => {
    vi.mocked(api.listMembers).mockRejectedValue(new Error("metadata store unreachable"));
    renderPanel();
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("metadata store unreachable")
    );
  });
});
