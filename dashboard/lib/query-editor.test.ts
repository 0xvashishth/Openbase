import { describe, expect, it } from "vitest";
import { editorConfigFor } from "./query-editor";

describe("editorConfigFor", () => {
  it("enables raw SQL for postgres + mysql", () => {
    expect(editorConfigFor("postgres").supportsRaw).toBe(true);
    expect(editorConfigFor("mysql").supportsRaw).toBe(true);
    expect(editorConfigFor("postgres").kind).toBe("sql");
  });

  it("disables raw for NoSQL engines with honest notes", () => {
    for (const e of ["ferretdb", "valkey", "arcadedb", "qdrant"]) {
      const cfg = editorConfigFor(e);
      expect(cfg.supportsRaw).toBe(false);
      expect(cfg.rawNote.length).toBeGreaterThan(0);
    }
    expect(editorConfigFor("ferretdb").kind).toBe("mongo");
    expect(editorConfigFor("valkey").kind).toBe("redis");
    expect(editorConfigFor("qdrant").kind).toBe("vector");
  });

  it("falls back gracefully for unknown engines", () => {
    expect(editorConfigFor(null).supportsRaw).toBe(false);
    expect(editorConfigFor("oracle").languageLabel).toBe("Query");
  });
});
