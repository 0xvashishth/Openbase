import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api", () => ({ api: { createProject: vi.fn() } }));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));

import { api } from "@/lib/api";
import { CreateProjectForm } from "./CreateProjectForm";

describe("CreateProjectForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // jsdom navigation: stub location.href setter
    Object.defineProperty(window, "location", {
      value: { href: "" },
      writable: true,
    });
  });

  it("creates a project scoped to the org and navigates to it", async () => {
    vi.mocked(api.createProject).mockResolvedValue({ id: "p1" } as never);
    const onCreated = vi.fn();
    const user = userEvent.setup();
    render(<CreateProjectForm orgId="o1" onCreated={onCreated} />);

    await user.type(screen.getByLabelText("Name"), "Shop");
    await user.type(screen.getByLabelText("Slug"), "shop");
    await user.click(screen.getByRole("button", { name: /create project/i }));

    expect(api.createProject).toHaveBeenCalledWith("tok", "o1", "Shop", "shop");
    expect(onCreated).toHaveBeenCalled();
    expect(window.location.href).toBe("/orgs/o1/projects/p1");
  });

  it("surfaces errors via alert", async () => {
    vi.mocked(api.createProject).mockRejectedValue(new Error("boom"));
    const user = userEvent.setup();
    render(<CreateProjectForm orgId="o1" />);

    await user.type(screen.getByLabelText("Name"), "Shop");
    await user.type(screen.getByLabelText("Slug"), "shop");
    await user.click(screen.getByRole("button", { name: /create project/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent("boom");
  });
});
