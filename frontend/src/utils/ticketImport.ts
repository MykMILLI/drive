import type {
  Blocker,
  ImportResult,
  ParsedTicket,
  Project,
  Quad,
  Task,
  TaskState,
} from "@/types/projects";
import { daysLeft } from "./projectLogic";

export function uid(): string {
  return Math.random().toString(36).slice(2, 9);
}

export function slugify(s: string): string {
  const x = s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  if (x.length <= 48) return x;
  const cut = x.slice(0, 48);
  const clean = x.charAt(48) === "-" ? cut : cut.slice(0, cut.lastIndexOf("-"));
  return clean.replace(/-+$/g, "");
}

/** Medium/High = important; due within 7 days (or overdue) = urgent. */
export function quadFor(t: ParsedTicket, today: Date = new Date()): Quad {
  const d = daysLeft(t.due, today);
  const important = /^(high|medium)$/i.test(t.priority);
  const urgent = d !== null && d <= 7;
  if (important && urgent) return "q1";
  if (important) return "q2";
  if (urgent) return "q3";
  return "q2";
}

/** Keyword-based suggestions. Phase 2 (Mavae) replaces these with model reasoning. */
export function suggestBlockers(text: string, owner: string): Blocker[] {
  const out: Blocker[] = [];
  if (/if necessary/i.test(text)) {
    out.push({ text: "Decision: is this actually needed?", owner, res: false });
  }
  if (/\baccess\b/i.test(text)) {
    out.push({
      text: "Access to every standalone tenant not yet confirmed",
      owner,
      res: false,
    });
  }
  return out;
}

export function newTask(
  text: string,
  date: string,
  st: TaskState,
  blockers: Blocker[]
): Task {
  return { id: uid(), text, date, st, blockers, links: [], cm: [] };
}

export function uniqueId(base: string, taken: string[]): string {
  let id = base || "project";
  let n = 2;
  while (taken.includes(id)) {
    id = base.slice(0, 60) + "-" + n;
    n++;
  }
  return id;
}

/**
 * Builds (or updates) a project from a parsed ticket. Re-importing the same
 * ticket number updates the project it created instead of duplicating it:
 * existing tasks (matched by text) and comments (matched by time + text) are kept.
 */
export function importTicket(
  t: ParsedTicket,
  existing: Project | undefined,
  takenIds: string[],
  today: Date = new Date()
): ImportResult {
  const quad = quadFor(t, today);
  const added = { tasks: 0, comments: 0, roadblocks: 0 };
  const isNew = !existing;
  const project: Project = existing
    ? existing
    : {
        id: uniqueId(slugify(t.id + "-" + t.title), takenIds),
        name: t.title,
        version: 0,
        parents: [],
        links: [],
        cm: [],
      };

  const prev = project.ticket;
  project.ticket = {
    id: t.id,
    category: t.category,
    priority: t.priority,
    kind: t.kind,
    due: t.due,
    created: t.created,
    assignee: t.assignee,
    desc: t.desc,
    linkExpires: t.linkExpires || (prev ? prev.linkExpires : 0),
    importedAt: Date.now(),
    url: prev ? prev.url : "",
  };

  let parent = project.parents.find((p) => p.fromTicket);
  if (!parent) {
    parent = {
      id: uid(),
      name: "Checklist - ticket #" + t.id,
      quad,
      owner: t.assignee,
      target: t.due,
      mode: "auto",
      tasks: [],
      links: [],
      cm: [],
      fromTicket: true,
    };
    project.parents.push(parent);
  }

  for (const text of t.items) {
    if (parent.tasks.some((x) => x.text === text)) continue;
    const blockers = suggestBlockers(text, t.assignee);
    parent.tasks.push(newTask(text, t.due, "todo", blockers));
    added.tasks++;
    added.roadblocks += blockers.length;
  }

  for (const c of t.entries) {
    const key = c.at + "|" + c.text;
    if (project.cm.some((x) => x.key === key)) continue;
    project.cm.push({
      id: uid(),
      key,
      by: c.by,
      at: c.at,
      text: c.text,
      activity: c.activity,
    });
    added.comments++;
  }
  project.cm.sort((a, b) => a.at - b.at);

  return { project, isNew, quad, added };
}
