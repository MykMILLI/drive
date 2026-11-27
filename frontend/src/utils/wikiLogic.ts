import type { WikiNode, WikiPage } from "@/types/wiki";
import { slugify, uniqueId } from "./ticketImport";

const byTitle = (a: WikiPage, b: WikiPage) =>
  a.title.localeCompare(b.title, undefined, { sensitivity: "base" });

/**
 * Flattens pages into display order: each page followed by its children,
 * siblings sorted by title. A page whose parent is missing (for example the
 * parent was removed by another client) is shown at the top level instead of
 * vanishing from the tree.
 */
export function buildTree(pages: WikiPage[]): WikiNode[] {
  const ids = new Set(pages.map((p) => p.id));
  const children = new Map<string, WikiPage[]>();
  for (const p of pages) {
    const key = p.parentId && ids.has(p.parentId) && p.parentId !== p.id ? p.parentId : "";
    const list = children.get(key);
    if (list) list.push(p);
    else children.set(key, [p]);
  }

  const out: WikiNode[] = [];
  const seen = new Set<string>();
  const walk = (parent: string, depth: number) => {
    for (const p of (children.get(parent) ?? []).sort(byTitle)) {
      if (seen.has(p.id)) continue; // guards against a cycle in bad data
      seen.add(p.id);
      out.push({ page: p, depth });
      walk(p.id, depth + 1);
    }
  };
  walk("", 0);
  return out;
}

/** Ids of every page nested anywhere under `id` (not including `id`). */
export function descendantIds(pages: WikiPage[], id: string): Set<string> {
  const out = new Set<string>();
  const queue = [id];
  while (queue.length) {
    const cur = queue.pop() as string;
    for (const p of pages) {
      if (p.parentId === cur && !out.has(p.id) && p.id !== id) {
        out.add(p.id);
        queue.push(p.id);
      }
    }
  }
  return out;
}

/** Breadcrumb trail from the top-level page down to (not including) `id`. */
export function ancestors(pages: WikiPage[], id: string): WikiPage[] {
  const byId = new Map(pages.map((p) => [p.id, p]));
  const trail: WikiPage[] = [];
  const seen = new Set<string>([id]);
  let cur = byId.get(id)?.parentId ?? "";
  while (cur && !seen.has(cur)) {
    const p = byId.get(cur);
    if (!p) break;
    seen.add(cur);
    trail.unshift(p);
    cur = p.parentId;
  }
  return trail;
}

/** Pages that document a given board card (parent item). */
export function pagesFor(pages: WikiPage[], parentItemId: string): WikiPage[] {
  return pages.filter((p) => p.refs.includes(parentItemId)).sort(byTitle);
}

export function newPageId(title: string, taken: string[]): string {
  return uniqueId(slugify(title) || "page", taken);
}

const escapeHtml = (s: string) =>
  s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

/**
 * Turns [[Page title]], [[page-id]] and [[target|label]] into links to other
 * pages of the same project (href "#wiki:<id>"), matching by id first and then
 * by title. A target that matches nothing is marked rather than dropped so the
 * author can see the link is broken. Code spans and fenced blocks are left
 * untouched so documentation can show the syntax.
 */
export function resolveWikiLinks(md: string, pages: WikiPage[]): string {
  const byId = new Map(pages.map((p) => [p.id, p]));
  const byName = new Map(pages.map((p) => [p.title.trim().toLowerCase(), p]));

  return md
    .split(/(```[\s\S]*?```|`[^`\n]*`)/)
    .map((chunk, i) => {
      if (i % 2 === 1) return chunk; // code
      return chunk.replace(/\[\[([^\[\]\n|]+)(?:\|([^\[\]\n]+))?\]\]/g, (_m, target: string, label?: string) => {
        const t = target.trim();
        const hit = byId.get(t) ?? byName.get(t.toLowerCase());
        const text = (label ?? hit?.title ?? t).trim();
        if (!hit) {
          return `<span class="wk-missing" title="No page named ${escapeHtml(t)}">${escapeHtml(text)}</span>`;
        }
        return `[${text.replace(/([\[\]\\])/g, "\\$1")}](#wiki:${hit.id})`;
      });
    })
    .join("");
}

/** Reads the page id out of an href produced by resolveWikiLinks. */
export function wikiIdFromHref(href: string | null | undefined): string {
  return href && href.startsWith("#wiki:") ? href.slice("#wiki:".length) : "";
}

/** First-draft content for a project's home page, seeded from its ticket. */
export function starterBody(project: {
  name: string;
  ticket?: { id: string; category: string; desc: string };
}): string {
  const lines = [`# ${project.name}`, ""];
  if (project.ticket) {
    lines.push(`Ticket #${project.ticket.id} · ${project.ticket.category}`, "");
    if (project.ticket.desc) lines.push(project.ticket.desc, "");
  }
  lines.push(
    "## Goal",
    "",
    "What this project is meant to achieve.",
    "",
    "## Notes",
    "",
    "Decisions, runbooks and anything the next person will need. Link other pages with `[[Page title]]`.",
    ""
  );
  return lines.join("\n");
}
