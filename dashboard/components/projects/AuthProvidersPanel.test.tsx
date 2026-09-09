import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api", () => ({
  api: {
    listAuthProviders: vi.fn(),
    upsertAuthProvider: vi.fn(),
    deleteAuthProvider: vi.fn(),
  },
  API_URL: "",
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));
vi.mock("@/components/ui/toast", () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn() }) }));

import { api } from "@/lib/api";
import { AuthProvidersPanel } from "./AuthProvidersPanel";

describe("AuthProvidersPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listAuthProviders).mockResolvedValue([]);
  });

  it("lists all four drivers as not configured", async () => {
    render(<AuthProvidersPanel projectId="p1" />);
    for (const label of ["GitHub", "Google", "Custom OIDC", "SMS (phone OTP)"]) {
      expect(await screen.findByText(label)).toBeInTheDocument();
    }
    expect((await screen.findAllByText("not configured")).length).toBe(4);
  });

  it("shows callback URLs for OAuth drivers but not SMS", async () => {
    render(<AuthProvidersPanel projectId="p1" />);
    await screen.findByText("GitHub");
    expect(screen.getAllByText(/Callback URL/).length).toBe(3);
  });

  it("saves a provider with secret and config", async () => {
    const user = userEvent.setup();
    vi.mocked(api.upsertAuthProvider).mockResolvedValue({ provider: "github", enabled: true });
    render(<AuthProvidersPanel projectId="p1" />);
    await screen.findByText("GitHub");
    await user.click(screen.getAllByRole("button", { name: "Set up" })[0]);
    await user.type(screen.getByLabelText("Client ID"), "cid");
    await user.type(screen.getByLabelText(/Client secret/), "shh");
    await user.click(screen.getByRole("button", { name: "Save provider" }));
    await waitFor(() =>
      expect(api.upsertAuthProvider).toHaveBeenCalledWith(
        "tok",
        "p1",
        expect.objectContaining({ provider: "github", client_id: "cid", client_secret: "shh" })
      )
    );
  });

  it("rejects invalid JSON config client-side", async () => {
    const user = userEvent.setup();
    render(<AuthProvidersPanel projectId="p1" />);
    await screen.findByText("GitHub");
    await user.click(screen.getAllByRole("button", { name: "Set up" })[0]);
    await user.clear(screen.getByLabelText("Config (JSON)"));
    await user.click(screen.getByLabelText("Config (JSON)"));
    await user.paste("{nope");
    await user.click(screen.getByRole("button", { name: "Save provider" }));
    expect(await screen.findByText("Config must be valid JSON")).toBeInTheDocument();
    expect(api.upsertAuthProvider).not.toHaveBeenCalled();
  });
});
