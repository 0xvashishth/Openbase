import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { GlobalSearch } from "./GlobalSearch";

const push = vi.fn();
let mockPath = "/orgs";

vi.mock("next/navigation", () => ({
  usePathname: () => mockPath,
  useRouter: () => ({ push }),
}));

vi.mock("@/lib/api", () => ({
  api: { listOrgs: vi.fn(), listProjects: vi.fn(), listCollections: vi.fn() },
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));

import { api } from "@/lib/api";

const orgs = [{ id: "o1", name: "Acme", slug: "acme-corp" }];

describe("GlobalSearch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockPath = "/orgs";
    vi.mocked(api.listOrgs).mockResolvedValue(orgs as never);
    vi.mocked(api.listProjects).mockResolvedValue([
      { id: "p1", name: "Shop", slug: "shop" },
    ] as never);
    vi.mocked(api.listCollections).mockResolvedValue([{ name: "users" }] as never);
  });

  async function openSearch() {
    const user = userEvent.setup();
    render(<GlobalSearch />);
    await user.click(screen.getByText(/search organizations, projects, tables/i));
    await waitFor(() => expect(api.listOrgs).toHaveBeenCalled());
    return user;
  }

  it("lists organizations and projects after opening", async () => {
    await openSearch();
    // accessible names join label + hint
    expect(await screen.findByRole("option", { name: "Acme acme-corp" })).toBeInTheDocument();
    expect(await screen.findByRole("option", { name: "Shop Acme" })).toBeInTheDocument();
  });

  it("filters results as you type", async () => {
    const user = await openSearch();
    await screen.findByRole("option", { name: "Shop Acme" });
    await user.type(screen.getByRole("combobox"), "shop");
    await waitFor(() =>
      expect(screen.queryByRole("option", { name: "Acme acme-corp" })).not.toBeInTheDocument()
    );
    expect(screen.getByRole("option", { name: "Shop Acme" })).toBeInTheDocument();
  });

  it("navigates on click", async () => {
    const user = await openSearch();
    await user.click(await screen.findByRole("option", { name: "Shop Acme" }));
    expect(push).toHaveBeenCalledWith("/orgs/o1/projects/p1");
  });

  it("navigates with keyboard (arrows + enter)", async () => {
    const user = await openSearch();
    await screen.findByRole("option", { name: "Shop Acme" });
    await user.type(screen.getByRole("combobox"), "shop");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(push).toHaveBeenCalledWith("/orgs/o1/projects/p1");
  });

  it("opens with Ctrl+K", async () => {
    const user = userEvent.setup();
    render(<GlobalSearch />);
    await user.keyboard("{Control>}k{/Control}");
    expect(await screen.findByRole("combobox")).toBeInTheDocument();
  });

  it("shows project tools and tables in project scope", async () => {
    mockPath = "/orgs/o1/projects/p1/tables";
    const user = userEvent.setup();
    render(<GlobalSearch />);
    await user.click(screen.getByText(/search organizations, projects, tables/i));
    expect(await screen.findByRole("option", { name: /sql editor/i })).toBeInTheDocument();
    expect(await screen.findByRole("option", { name: /users/i })).toBeInTheDocument();
  });
});
