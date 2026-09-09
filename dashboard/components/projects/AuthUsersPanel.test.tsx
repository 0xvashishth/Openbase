import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api", () => ({
  api: {
    listProjectUsers: vi.fn(),
    updateProjectUser: vi.fn(),
    deleteProjectUser: vi.fn(),
  },
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));
vi.mock("@/components/ui/toast", () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn() }) }));

import { api } from "@/lib/api";
import { AuthUsersPanel } from "./AuthUsersPanel";

const users = [
  { id: "u1", email: "a@example.com", email_confirmed_at: "2026-01-01T00:00:00Z", created_at: "" },
  { id: "u2", email: "b@example.com", created_at: "" },
];

describe("AuthUsersPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listProjectUsers).mockResolvedValue({ users, page: 1, per_page: 50 });
  });

  it("lists users with confirmation badges", async () => {
    render(<AuthUsersPanel projectId="p1" />);
    expect(await screen.findByText("a@example.com")).toBeInTheDocument();
    expect(screen.getByText("confirmed")).toBeInTheDocument();
    expect(screen.getByText("unconfirmed")).toBeInTheDocument();
  });

  it("shows an empty state before any signup", async () => {
    vi.mocked(api.listProjectUsers).mockResolvedValue({ users: [], page: 1, per_page: 50 });
    render(<AuthUsersPanel projectId="p1" />);
    expect(await screen.findByText("No end users yet")).toBeInTheDocument();
  });

  it("searches by email", async () => {
    const user = userEvent.setup();
    render(<AuthUsersPanel projectId="p1" />);
    await screen.findByText("a@example.com");
    await user.type(screen.getByLabelText("Search by email"), "a@");
    await user.click(screen.getByRole("button", { name: "Search" }));
    await waitFor(() => expect(api.listProjectUsers).toHaveBeenLastCalledWith("tok", "p1", "a@"));
  });

  it("bans and deletes with confirmation", async () => {
    const user = userEvent.setup();
    vi.mocked(api.updateProjectUser).mockResolvedValue({ user: users[0] });
    vi.mocked(api.deleteProjectUser).mockResolvedValue({ status: "ok" });
    render(<AuthUsersPanel projectId="p1" />);
    await screen.findByText("a@example.com");
    await user.click(screen.getAllByRole("button", { name: "Ban" })[0]);
    await waitFor(() =>
      expect(api.updateProjectUser).toHaveBeenCalledWith("tok", "p1", "u1", { banned: true, ban_duration: "720h" })
    );
    await user.click(screen.getAllByRole("button", { name: "Delete" })[0]);
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(api.deleteProjectUser).toHaveBeenCalledWith("tok", "p1", "u1"));
  });

  it("surfaces load failures inline", async () => {
    vi.mocked(api.listProjectUsers).mockRejectedValue(new Error("boom"));
    render(<AuthUsersPanel projectId="p1" />);
    expect(await screen.findByText("boom")).toBeInTheDocument();
  });
});
