import { describe, expect, it } from "vitest";
import { editorConfigFor } from "./query-editor";

describe("editorConfigFor", () => {
  it("enables raw SQL for postgres + mysql", () => {
    expect(editorConfigFor("postgres").supportsRaw).toBe(true);
    expect(editorConfigFor("mysql").supportsRaw).toBe(true);
    expect(editorConfigFor("postgres").kind).toBe("sql");
  });

  it("enables raw execution for NoSQL engines with native-language notes", () => {
    for (const e of ["ferretdb", "valkey", "arcadedb", "qdrant"]) {
      const cfg = editorConfigFor(e);
      expect(cfg.supportsRaw).toBe(true);
      expect(cfg.rawNote.length).toBeGreaterThan(0);
    }
    expect(editorConfigFor("ferretdb").kind).toBe("mongo");
    expect(editorConfigFor("valkey").kind).toBe("redis");
    expect(editorConfigFor("qdrant").kind).toBe("vector");
    expect(editorConfigFor("arcadedb").kind).toBe("arcade");
  });

  it("seeds each engine's sample with its own native syntax", () => {
    expect(editorConfigFor("postgres").sample).toContain("SELECT");
    expect(editorConfigFor("ferretdb").sample).toContain("db.");
    expect(editorConfigFor("valkey").sample).toContain("KEYS");
    expect(editorConfigFor("arcadedb").sample).toContain("SELECT FROM");
    expect(editorConfigFor("qdrant").sample).toContain("SCROLL");
  });

  it("falls back gracefully for unknown engines", () => {
    expect(editorConfigFor(null).supportsRaw).toBe(false);
    expect(editorConfigFor("oracle").languageLabel).toBe("Query");
  });
});
