import { describe, expect, it } from "vitest";
import { normalizeTool, parseRoute, projectToolPath } from "./nav";

describe("parseRoute", () => {
  it("treats org list + global projects as platform scope", () => {
    expect(parseRoute("/orgs").scope).toBe("platform");
    expect(parseRoute("/projects").scope).toBe("platform");
    expect(parseRoute("/").scope).toBe("platform");
  });

  it("parses org scope without leaking project ids", () => {
    const r = parseRoute("/orgs/org-1");
    expect(r).toMatchObject({ scope: "org", orgId: "org-1", projectId: null });
  });

  it("parses project scope with tool defaulting to overview", () => {
    expect(parseRoute("/orgs/o1/projects/p1")).toMatchObject({
      scope: "project",
      orgId: "o1",
      projectId: "p1",
      tool: "overview",
    });
    expect(parseRoute("/orgs/o1/projects/p1/tables").tool).toBe("tables");
  });

  it("treats auth routes separately", () => {
    expect(parseRoute("/login").scope).toBe("auth");
    expect(parseRoute("/register").scope).toBe("auth");
  });
});

describe("projectToolPath + normalizeTool", () => {
  it("builds overview path without suffix", () => {
    expect(projectToolPath("o", "p", "overview")).toBe("/orgs/o/projects/p");
  });

  it("builds tool paths", () => {
    expect(projectToolPath("o", "p", "tables")).toBe("/orgs/o/projects/p/tables");
  });

  it("normalizes legacy aliases", () => {
    expect(normalizeTool("data")).toBe("tables");
    expect(normalizeTool("api-keys")).toBe("api");
    expect(normalizeTool(null)).toBe("overview");
  });
});
