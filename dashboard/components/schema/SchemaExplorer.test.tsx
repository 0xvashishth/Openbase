import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SchemaExplorer } from "./SchemaExplorer";

vi.mock("@/lib/api", () => ({ api: { getFullSchema: vi.fn() } }));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));

// ReactFlow needs ResizeObserver + DOM measurements; stub for jsdom.
class RO {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal("ResizeObserver", RO);
Object.defineProperty(HTMLElement.prototype, "getBoundingClientRect", {
  configurable: true,
  value: () => ({ width: 800, height: 500, top: 0, left: 0, right: 800, bottom: 500, x: 0, y: 0, toJSON: () => ({}) }),
});

import { api } from "@/lib/api";

const caps = {
  supports_relational_joins: true,
  supports_foreign_keys: true,
  supports_native_triggers: false,
  supports_change_streams: false,
  supports_realtime: "none",
  supports_transactions: true,
  supports_full_text_search: false,
  supports_vector_search: false,
};

describe("SchemaExplorer crash regression", () => {
  beforeEach(() => vi.clearAllMocks());

  it("does not crash when backend returns null collections/relationships", async () => {
    vi.mocked(api.getFullSchema).mockResolvedValue({
      collections: null,
      relationships: null,
      capabilities: caps,
    } as never);
    render(<SchemaExplorer projectId="p1" engine="postgres" />);
    await waitFor(() => expect(screen.getByText(/no tables found/i)).toBeInTheDocument());
  });

  it("renders tables + filters by name", async () => {
    vi.mocked(api.getFullSchema).mockResolvedValue({
      collections: [
        { collection: "users", columns: [{ name: "id", data_type: "uuid", is_primary: true }], indexes: [] },
        { collection: "orders", columns: [{ name: "id", data_type: "uuid", is_primary: true }], indexes: [] },
      ],
      relationships: [
        { from_collection: "orders", from_column: "user_id", to_collection: "users", to_column: "id" },
      ],
      capabilities: caps,
    } as never);
    const user = userEvent.setup();
    render(<SchemaExplorer projectId="p1" engine="postgres" />);
    await waitFor(() => expect(screen.getByText("2 tables")).toBeInTheDocument());
    await user.type(screen.getByLabelText(/filter tables/i), "user");
    expect(screen.queryByText(/no tables match/i)).not.toBeInTheDocument();
    await user.clear(screen.getByLabelText(/filter tables/i));
    await user.type(screen.getByLabelText(/filter tables/i), "zzz");
    expect(screen.getByText(/no tables match/i)).toBeInTheDocument();
  });
});
