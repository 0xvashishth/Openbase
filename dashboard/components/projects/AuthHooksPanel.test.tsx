import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api", () => ({
  api: {
    listAuthHooks: vi.fn(),
    upsertAuthHook: vi.fn(),
    deleteAuthHook: vi.fn(),
    listFunctions: vi.fn(),
  },
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));
vi.mock("@/components/ui/toast", () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn() }) }));

import { api } from "@/lib/api";
import { AuthHooksPanel } from "./AuthHooksPanel";

import type { Function } from "@/lib/types";

const fns: Function[] = [{ id: "f1", project_id: "p1", name: "gate", runtime: "node", source: "", created_at: "" }];

describe("AuthHooksPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listAuthHooks).mockResolvedValue([]);
    vi.mocked(api.listFunctions).mockResolvedValue(fns);
  });

  it("shows an empty state with no hooks", async () => {
    render(<AuthHooksPanel projectId="p1" />);
    expect(await screen.findByText("No auth hooks")).toBeInTheDocument();
  });

  it("lists hooks with fail-open badges", async () => {
    vi.mocked(api.listAuthHooks).mockResolvedValue([
      { id: "h1", event: "before-user-created", function_id: "f1", fail_open: true, created_at: "" },
    ]);
    render(<AuthHooksPanel projectId="p1" />);
    expect((await screen.findAllByText("before-user-created")).length).toBeGreaterThan(0);
    expect(screen.getByText("fail-open")).toBeInTheDocument();
    expect(screen.getByText(/→ gate/)).toBeInTheDocument();
  });

  it("saves a hook for the chosen event and function", async () => {
    const user = userEvent.setup();
    vi.mocked(api.upsertAuthHook).mockResolvedValue({
      id: "h1",
      event: "before-token-issued",
      function_id: "f1",
      fail_open: false,
      created_at: "",
    });
    render(<AuthHooksPanel projectId="p1" />);
    await screen.findByText("No auth hooks");
    await user.selectOptions(screen.getByLabelText("Event"), "before-token-issued");
    await user.click(screen.getByRole("button", { name: "Save hook" }));
    await waitFor(() =>
      expect(api.upsertAuthHook).toHaveBeenCalledWith(
        "tok",
        "p1",
        expect.objectContaining({ event: "before-token-issued", function_id: "f1" })
      )
    );
  });

  it("requires a function before saving", async () => {
    const user = userEvent.setup();
    vi.mocked(api.listFunctions).mockResolvedValue([]);
    render(<AuthHooksPanel projectId="p1" />);
    await screen.findByText("No auth hooks");
    await user.click(screen.getByRole("button", { name: "Save hook" }));
    expect(await screen.findByText(/Pick a function/)).toBeInTheDocument();
    expect(api.upsertAuthHook).not.toHaveBeenCalled();
  });
});
