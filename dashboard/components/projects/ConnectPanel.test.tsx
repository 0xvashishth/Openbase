import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockUseProject = vi.fn();

vi.mock("@/lib/project-context", () => ({ useProject: () => mockUseProject() }));
vi.mock("@/lib/api", () => ({
  api: {
    listAPIKeys: vi.fn(),
    createAPIKey: vi.fn(),
    getConnectInfo: vi.fn(),
    listCollections: vi.fn(),
  },
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));

import { api } from "@/lib/api";
import { ConnectPanel } from "./ConnectPanel";
import type { ConnectInfo } from "@/lib/types";

const ctx = {
  orgId: "o1",
  projectId: "p1",
  engine: "postgres",
  hasConnection: true,
  supportsRealtime: true,
};

const connectInfo: ConnectInfo = {
  project_id: "p1",
  has_connection: true,
  engine: "postgres",
  mode: "byodb",
  status: "connected",
  api_base_url: "https://api.example.com",
  realtime_url: "wss://api.example.com/v1/realtime",
  endpoints: {
    list_tables: "GET /v1/api/tables",
    query_rows: "GET /v1/api/{collection}?limit=&offset=&order_by=&order=",
    table_schema: "GET /v1/api/{collection}/_schema",
    insert_row: "POST /v1/api/{collection}",
    update_row: "PUT /v1/api/{collection}/{id}",
    delete_row: "DELETE /v1/api/{collection}/{id}",
    realtime: "GET /v1/realtime",
  },
  auth_header: "Authorization: Bearer ob_...",
  supports_realtime: "native",
  active_api_keys: 0,
};

/** The client-library tab panel (the only one rendered by Radix at a time). */
function snippetPanel() {
  return screen.getByRole("tabpanel");
}

describe("ConnectPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseProject.mockReturnValue(ctx);
    vi.mocked(api.listAPIKeys).mockResolvedValue([]);
    vi.mocked(api.getConnectInfo).mockResolvedValue(connectInfo);
    vi.mocked(api.listCollections).mockResolvedValue([]);
    Object.defineProperty(window, "location", {
      value: { origin: "https://dash.example.com" },
      writable: true,
    });
  });

  it("prefers the server-advertised API + realtime URLs", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText("https://api.example.com")).toBeInTheDocument();
    expect(screen.getByText("wss://api.example.com/v1/realtime")).toBeInTheDocument();
    expect(screen.getByText("p1")).toBeInTheDocument();
  });

  it("falls back to the dashboard origin when connect-info fails", async () => {
    vi.mocked(api.getConnectInfo).mockRejectedValue(new Error("nope"));
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText("https://dash.example.com")).toBeInTheDocument();
    expect(screen.getByText("wss://dash.example.com/v1/realtime")).toBeInTheDocument();
  });

  it("renders the endpoint reference from the server", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText("GET /v1/api/tables")).toBeInTheDocument();
    expect(screen.getByText("PUT /v1/api/{collection}/{id}")).toBeInTheDocument();
    expect(screen.getByText("Authorization: Bearer ob_...")).toBeInTheDocument();
  });

  it("offers every client language as a tab", async () => {
    render(<ConnectPanel projectId="p1" />);
    for (const label of ["cURL", "JavaScript", "TypeScript / Next.js", "Python", "Go"]) {
      expect(await screen.findByRole("tab", { name: label })).toBeInTheDocument();
    }
  });

  it("switches between language snippets", async () => {
    const user = userEvent.setup();
    render(<ConnectPanel projectId="p1" />);
    await screen.findByRole("tab", { name: "cURL" });
    expect(snippetPanel()).toHaveTextContent("curl");

    await user.click(screen.getByRole("tab", { name: "Python" }));
    expect(snippetPanel()).toHaveTextContent("import httpx");

    await user.click(screen.getByRole("tab", { name: "Go" }));
    expect(snippetPanel()).toHaveTextContent("os.Getenv");

    await user.click(screen.getByRole("tab", { name: "TypeScript / Next.js" }));
    expect(snippetPanel()).toHaveTextContent("lib/openbase.ts");
  });

  it("uses a real table name in snippets when collections are known", async () => {
    vi.mocked(api.listCollections).mockResolvedValue([{ name: "orders" }, { name: "users" }] as never);
    render(<ConnectPanel projectId="p1" />);
    await waitFor(() => expect(snippetPanel()).toHaveTextContent("/v1/api/orders"));
    expect(screen.queryByText(/replace your_table/i)).not.toBeInTheDocument();
  });

  it("lets the user pick which table the snippets use", async () => {
    vi.mocked(api.listCollections).mockResolvedValue([{ name: "orders" }, { name: "users" }] as never);
    const user = userEvent.setup();
    render(<ConnectPanel projectId="p1" />);
    const picker = await screen.findByLabelText("Example table");
    await user.selectOptions(picker, "users");
    expect(snippetPanel()).toHaveTextContent("/v1/api/users");
  });

  it("falls back to a placeholder table with a hint when no collections load", async () => {
    render(<ConnectPanel projectId="p1" />);
    await waitFor(() => expect(snippetPanel()).toHaveTextContent("your_table"));
    expect(screen.getByText(/replace/i)).toBeInTheDocument();
    expect(screen.queryByLabelText("Example table")).not.toBeInTheDocument();
  });

  it("uses a placeholder key until one is created", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/no active keys yet/i)).toBeInTheDocument();
    expect(snippetPanel()).toHaveTextContent("ob_your_api_key");
  });

  it("reports the server's active key count", async () => {
    vi.mocked(api.getConnectInfo).mockResolvedValue({ ...connectInfo, active_api_keys: 2 });
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/2 active keys/i)).toBeInTheDocument();
  });

  it("creates a key and shows it once, inlined into snippets", async () => {
    vi.mocked(api.createAPIKey).mockResolvedValue({ plaintext: "ob_live_key" } as never);
    const user = userEvent.setup();
    render(<ConnectPanel projectId="p1" />);

    await user.click(await screen.findByRole("button", { name: /create api key/i }));

    await waitFor(() => expect(api.createAPIKey).toHaveBeenCalledWith("tok", "p1", "connect-quickstart"));
    expect(await screen.findByText("ob_live_key")).toBeInTheDocument();
    expect(snippetPanel()).toHaveTextContent("Bearer ob_live_key");
  });

  it("warns that the key must stay server-side", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/keep the key server-side/i)).toBeInTheDocument();
    expect(screen.getByText(/OPENBASE_API_KEY=ob_your_api_key/)).toBeInTheDocument();
  });

  it("shows a realtime snippet for native engines", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByRole("button", { name: "Copy Realtime example" })).toBeInTheDocument();
  });

  it("replaces the realtime snippet with an honest empty state when unsupported", async () => {
    vi.mocked(api.getConnectInfo).mockResolvedValue({ ...connectInfo, supports_realtime: "polling" });
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/realtime unavailable on this engine/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Copy Realtime example" })).not.toBeInTheDocument();
    expect(screen.getByText(/no native change stream/i)).toBeInTheDocument();
  });

  it("explains ORM access for a bring-your-own database", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/bring-your-own database/i)).toBeInTheDocument();
  });

  it("explains that provisioned credentials stay server-side", async () => {
    vi.mocked(api.getConnectInfo).mockResolvedValue({ ...connectInfo, mode: "provisioned" });
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/does not expose them/i)).toBeInTheDocument();
  });

  it("points at DB Source when the project has no database", async () => {
    vi.mocked(api.getConnectInfo).mockResolvedValue({
      ...connectInfo,
      has_connection: false,
      engine: undefined,
      mode: undefined,
      status: undefined,
      supports_realtime: undefined,
    });
    mockUseProject.mockReturnValue({ ...ctx, hasConnection: false, engine: null });
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/no database attached yet/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /go to db source/i })).toHaveAttribute(
      "href",
      "/orgs/o1/projects/p1/db-source"
    );
  });

  it("surfaces key-loading failures without breaking the page", async () => {
    vi.mocked(api.listAPIKeys).mockRejectedValue(new Error("boom"));
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByRole("alert")).toHaveTextContent("boom");
    expect(screen.getByText("https://api.example.com")).toBeInTheDocument();
  });

  it("links to the API Keys tab for management", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByRole("link", { name: "API Keys" })).toHaveAttribute(
      "href",
      "/orgs/o1/projects/p1/api"
    );
  });
});
