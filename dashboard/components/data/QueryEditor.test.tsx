import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryEditor } from "./QueryEditor";

vi.mock("@/lib/api", () => ({
  api: { execSQL: vi.fn(), queryRows: vi.fn(), listCollections: vi.fn() },
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));
vi.mock("next-themes", () => ({ useTheme: () => ({ resolvedTheme: "light" }) }));

import { api } from "@/lib/api";

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

  it("runs FerretDB via raw mongo-shell (not the query builder)", async () => {
    vi.mocked(api.execSQL).mockResolvedValue({ columns: ["_id"], rows: [{ _id: "x" }] } as never);
    const user = userEvent.setup();
    render(<QueryEditor projectId="p1" engine="ferretdb" />);
    expect(screen.getByText(/MongoDB/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /run/i }));
    await waitFor(() => expect(api.execSQL).toHaveBeenCalledWith("tok", "p1", expect.stringContaining("db.")));
    expect(screen.getByRole("table")).toHaveTextContent("x");
  });

  it("runs Valkey via raw Redis commands", async () => {
    vi.mocked(api.execSQL).mockResolvedValue({ columns: ["result"], rows: [{ result: "OK" }] } as never);
    const user = userEvent.setup();
    render(<QueryEditor projectId="p1" engine="valkey" />);
    expect(screen.getByText("Valkey (Redis)")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /run/i }));
    await waitFor(() => expect(api.execSQL).toHaveBeenCalled());
  });

  it("renders write results (affected_rows)", async () => {
    vi.mocked(api.execSQL).mockResolvedValue(
      { columns: ["affected_rows"], rows: [{ affected_rows: 3 }] } as never
    );
    const user = userEvent.setup();
    render(<QueryEditor projectId="p1" engine="postgres" />);
    await user.click(screen.getByRole("button", { name: /run/i }));
    expect(await screen.findByRole("table")).toHaveTextContent("3");
  });
});
