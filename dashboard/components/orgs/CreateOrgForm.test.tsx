import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));

vi.mock("@/lib/api", () => ({ api: { createOrg: vi.fn() } }));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));

import { api } from "@/lib/api";
import { CreateOrgForm } from "./CreateOrgForm";

describe("CreateOrgForm", () => {
  beforeEach(() => vi.clearAllMocks());

  it("requires name + slug and submits to the API", async () => {
    vi.mocked(api.createOrg).mockResolvedValue({ id: "o1" } as never);
    const onCreated = vi.fn();
    const user = userEvent.setup();
    render(<CreateOrgForm onCreated={onCreated} />);

    await user.type(screen.getByLabelText("Name"), "Acme");
    await user.type(screen.getByLabelText("Slug"), "acme");
    await user.click(screen.getByRole("button", { name: /create organization/i }));

    expect(api.createOrg).toHaveBeenCalledWith("tok", "Acme", "acme");
    expect(onCreated).toHaveBeenCalled();
    expect(push).toHaveBeenCalledWith("/orgs/o1");
  });

  it("shows API errors without navigating", async () => {
    vi.mocked(api.createOrg).mockRejectedValue(new Error("slug taken"));
    const user = userEvent.setup();
    render(<CreateOrgForm />);

    await user.type(screen.getByLabelText("Name"), "Acme");
    await user.type(screen.getByLabelText("Slug"), "acme");
    await user.click(screen.getByRole("button", { name: /create organization/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent("slug taken");
    expect(push).not.toHaveBeenCalled();
  });
});
