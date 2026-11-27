import { describe, expect, it } from "vitest";
import type { WikiPage } from "@/types/wiki";
import {
  ancestors,
  buildTree,
  descendantIds,
  newPageId,
  pagesFor,
  resolveWikiLinks,
  starterBody,
  wikiIdFromHref,
} from "./wikiLogic";

const page = (id: string, title: string, parentId = "", refs: string[] = []): WikiPage => ({
  id,
  title,
  parentId,
  version: 1,
  refs,
});

const PAGES = [
  page("home", "Home"),
  page("runbooks", "Runbooks", "home"),
  page("vpn", "VPN setup", "runbooks"),
  page("backup", "Backup restore", "runbooks"),
  page("faq", "FAQ", "home"),
];

describe("buildTree", () => {
  it("orders children under their parent, siblings by title", () => {
    expect(buildTree(PAGES).map((n) => [n.page.id, n.depth])).toEqual([
      ["home", 0],
      ["faq", 1],
      ["runbooks", 1],
      ["backup", 2],
      ["vpn", 2],
    ]);
  });

  it("shows a page whose parent is missing at the top level", () => {
    const tree = buildTree([page("a", "A", "gone"), page("b", "B")]);
    expect(tree.map((n) => [n.page.id, n.depth])).toEqual([
      ["a", 0],
      ["b", 0],
    ]);
  });

  it("does not loop or drop pages when the data contains a cycle", () => {
    const tree = buildTree([page("a", "A", "b"), page("b", "B", "a"), page("c", "C")]);
    expect(tree.map((n) => n.page.id)).toEqual(["c"]);
  });
});

describe("descendantIds / ancestors / pagesFor", () => {
  it("collects every nested page", () => {
    expect([...descendantIds(PAGES, "home")].sort()).toEqual(["backup", "faq", "runbooks", "vpn"]);
    expect([...descendantIds(PAGES, "vpn")]).toEqual([]);
  });

  it("builds a breadcrumb from the top down", () => {
    expect(ancestors(PAGES, "vpn").map((p) => p.id)).toEqual(["home", "runbooks"]);
    expect(ancestors(PAGES, "home")).toEqual([]);
  });

  it("finds the pages documenting a board card", () => {
    const pages = [page("a", "Zed", "", ["p1"]), page("b", "Alpha", "", ["p1", "p2"]), page("c", "C")];
    expect(pagesFor(pages, "p1").map((p) => p.id)).toEqual(["b", "a"]);
    expect(pagesFor(pages, "none")).toEqual([]);
  });
});

describe("newPageId", () => {
  it("makes a slug and avoids ids already taken", () => {
    expect(newPageId("VPN Setup!", [])).toBe("vpn-setup");
    expect(newPageId("VPN Setup!", ["vpn-setup"])).toBe("vpn-setup-2");
  });

  it("falls back to a usable id when the title has no letters or digits", () => {
    expect(newPageId("???", [])).toBe("page");
  });

  it("always satisfies the server's id rule", () => {
    const re = /^[a-z0-9][a-z0-9-]{0,63}$/;
    const long = "word ".repeat(40);
    const taken: string[] = [];
    for (let i = 0; i < 5; i++) {
      const id = newPageId(long, taken);
      expect(id).toMatch(re);
      taken.push(id);
    }
  });
});

describe("resolveWikiLinks", () => {
  it("links by title, id and custom label", () => {
    const md = "See [[VPN setup]], [[faq]] and [[backup|the restore guide]].";
    expect(resolveWikiLinks(md, PAGES)).toBe(
      "See [VPN setup](#wiki:vpn), [FAQ](#wiki:faq) and [the restore guide](#wiki:backup)."
    );
  });

  it("matches titles case-insensitively", () => {
    expect(resolveWikiLinks("[[vpn SETUP]]", PAGES)).toBe("[VPN setup](#wiki:vpn)");
  });

  it("marks a broken link instead of dropping it, and escapes the name", () => {
    const out = resolveWikiLinks("[[<img src=x>]]", PAGES);
    expect(out).toContain('class="wk-missing"');
    expect(out).not.toContain("<img");
  });

  it("leaves code spans and fenced blocks alone", () => {
    const md = "Use `[[Home]]` to link.\n\n```\n[[Home]]\n```\n\nThen [[Home]].";
    const out = resolveWikiLinks(md, PAGES);
    expect(out).toContain("`[[Home]]`");
    expect(out).toContain("```\n[[Home]]\n```");
    expect(out.endsWith("Then [Home](#wiki:home).")).toBe(true);
  });

  it("round-trips the id through wikiIdFromHref", () => {
    expect(wikiIdFromHref("#wiki:vpn")).toBe("vpn");
    expect(wikiIdFromHref("https://example.com")).toBe("");
    expect(wikiIdFromHref(null)).toBe("");
  });
});

describe("starterBody", () => {
  it("titles the page with the project name", () => {
    expect(starterBody({ name: "Tenant migration" }).startsWith("# Tenant migration\n")).toBe(true);
  });

  it("carries the ticket details when the project came from one", () => {
    const body = starterBody({
      name: "Tenant migration",
      ticket: { id: "1552", category: "GENERAL", desc: "Move every office to one tenant." },
    });
    expect(body).toContain("Ticket #1552 · GENERAL");
    expect(body).toContain("Move every office to one tenant.");
  });
});
