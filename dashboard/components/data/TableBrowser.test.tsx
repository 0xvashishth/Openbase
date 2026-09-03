import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TableBrowser, cellText } from "./TableBrowser";

vi.mock("@/lib/api", () => ({
  api: { listCollections: vi.fn(), getSchema: vi.fn(), queryRows: vi.fn() },
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));

import { api } from "@/lib/api";

describe("cellText", () => {
  it("renders NULL for nullish and JSON for objects", () => {
    expect(cellText(null)).toBe("NULL");
    expect(cellText({ a: 1 })).toBe('{"a":1}');
  });
});

describe("TableBrowser", () => {
  beforeEach(() => vi.clearAllMocks());

  it("shows empty state when no collections (null-safe)", async () => {
    vi.mocked(api.listCollections).mockResolvedValue(null as never);
    render(<TableBrowser projectId="p1" />);
    await waitFor(() => expect(screen.getByText(/no tables found/i)).toBeInTheDocument());
  });

  it("paginates + sorts + filters via queryRows params", async () => {
    vi.mocked(api.listCollections).mockResolvedValue([{ name: "users" }] as never);
    vi.mocked(api.getSchema).mockResolvedValue({
      collection: "users",
      columns: [{ name: "id" }, { name: "status" }],
    } as never);
    vi.mocked(api.queryRows).mockResolvedValue({
      columns: ["id", "status"],
      rows: Array.from({ length: 25 }, (_, i) => ({ id: i + 1, status: "active" })),
    } as never);
    const user = userEvent.setup();
    render(<TableBrowser projectId="p1" />);
    await user.click(await screen.findByText("users"));
    await waitFor(() => expect(api.queryRows).toHaveBeenCalled());
    expect(screen.getAllByText("active").length).toBeGreaterThan(0);

    // sort by status
    await user.click(screen.getByLabelText("Sort by status"));
    await waitFor(() =>
      expect(api.queryRows).toHaveBeenLastCalledWith(
        "tok",
        "p1",
        expect.objectContaining({ order_by: [{ field: "status", desc: false }] })
      )
    );

    // next page sends offset
    await user.click(screen.getByRole("button", { name: /next/i }));
    expect(api.queryRows).toHaveBeenLastCalledWith(
      "tok",
      "p1",
      expect.objectContaining({ offset: 25, limit: 25 })
    );

    // filter sends condition
    await user.type(screen.getByLabelText("Field"), "status");
    await user.type(screen.getByLabelText("Value"), "active");
    await user.click(screen.getByRole("button", { name: "Apply" }));
    expect(api.queryRows).toHaveBeenLastCalledWith(
      "tok",
      "p1",
      expect.objectContaining({ conditions: [{ field: "status", operator: "eq", value: "active" }] })
    );
  });
});
