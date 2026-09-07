import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiStatus } from "./ApiStatus";

vi.mock("@/lib/api", () => ({ api: { health: vi.fn(), me: vi.fn(), listOrgs: vi.fn() } }));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));

import { api } from "@/lib/api";

describe("ApiStatus", () => {
  beforeEach(() => vi.clearAllMocks());

  it("starts as checking, then reports operational with latency", async () => {
    vi.mocked(api.health).mockResolvedValue({ status: "ok" } as never);
    vi.mocked(api.me).mockResolvedValue({} as never);
    vi.mocked(api.listOrgs).mockResolvedValue([] as never);
    render(<ApiStatus />);
    expect(screen.getByRole("status")).toHaveAccessibleName(/checking/i);
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveAccessibleName(/operational/i)
    );
    expect(screen.getByText(/operational/i)).toBeInTheDocument();
  });

  it("reports unreachable when all checks fail (never flashes it early)", async () => {
    vi.mocked(api.health).mockRejectedValue(new Error("down"));
    vi.mocked(api.me).mockRejectedValue(new Error("down"));
    vi.mocked(api.listOrgs).mockRejectedValue(new Error("down"));
    render(<ApiStatus />);
    // starts neutral…
    expect(screen.getByRole("status")).toHaveAccessibleName(/checking/i);
    // …only flips after the checks actually settle
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveAccessibleName(/unreachable/i)
    );
  });

  it("reports degraded when only one API answers", async () => {
    vi.mocked(api.health).mockResolvedValue({ status: "ok" } as never);
    vi.mocked(api.me).mockResolvedValue({} as never);
    vi.mocked(api.listOrgs).mockRejectedValue(new Error("down"));
    render(<ApiStatus />);
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveAccessibleName(/degraded/i)
    );
  });
});
