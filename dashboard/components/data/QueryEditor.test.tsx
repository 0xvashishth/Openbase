import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryEditor, parseFilterJson } from "./QueryEditor";

vi.mock("@/lib/api", () => ({
  api: { execSQL: vi.fn(), queryRows: vi.fn(), listCollections: vi.fn() },
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));
vi.mock("next-themes", () => ({ useTheme: () => ({ resolvedTheme: "light" }) }));

import { api } from "@/lib/api";

describe("parseFilterJson", () => {
  it("parses flat objects to equality conditions", () => {
    expect(parseFilterJson('{ "status": "active", "n": 3 }')).toEqual([
      { field: "status", operator: "eq", value: "active" },
      { field: "n", operator: "eq", value: 3 },
    ]);
  });

  it("strips // comment lines so samples run", () => {
    expect(parseFilterJson('// hello\n{ "a": 1 }')).toEqual([{ field: "a", operator: "eq", value: 1 }]);
  });

  it("rejects non-objects", () => {
    expect(() => parseFilterJson("[1,2]")).toThrow();
  });
});

describe("QueryEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.mocked(api.listCollections).mockResolvedValue([{ name: "users" }] as never);
  });

  it("runs raw SQL on postgres and shows rows", async () => {
    vi.mocked(api.execSQL).mockResolvedValue({ columns: ["id"], rows: [{ id: 1 }] } as never);
    const user = userEvent.setup();
    render(<QueryEditor projectId="p1" engine="postgres" />);
    expect(screen.getByText(/PostgreSQL/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /run/i }));
    await waitFor(() => expect(api.execSQL).toHaveBeenCalledWith("tok", "p1", expect.stringContaining("SELECT")));
    expect(screen.getByRole("table")).toHaveTextContent("1");
  });

  it("surfaces SQL errors via alert", async () => {
    vi.mocked(api.execSQL).mockRejectedValue(new Error("syntax error near FROM"));
    const user = userEvent.setup();
    render(<QueryEditor projectId="p1" engine="postgres" />);
    await user.click(screen.getByRole("button", { name: /run/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/syntax error/);
  });

  it("runs NoSQL via query builder with collection + filter", async () => {
    vi.mocked(api.queryRows).mockResolvedValue({ columns: ["id"], rows: [{ id: "x" }] } as never);
    const user = userEvent.setup();
    render(<QueryEditor projectId="p1" engine="ferretdb" />);
    expect(screen.getByText(/MongoDB/)).toBeInTheDocument();
    // wait for collections to load into the picker before running
    await waitFor(() => expect(screen.getByLabelText(/collection/i)).toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: /run/i }));
    await waitFor(() => expect(api.queryRows).toHaveBeenCalledWith("tok", "p1", expect.objectContaining({ collection: "users" })));
  });
});
