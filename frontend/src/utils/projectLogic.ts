import type {
  Parent,
  ParentDisplay,
  Project,
  Task,
  TaskDisplay,
} from "@/types/projects";

export function openBlockers(t: Task): number {
  return t.blockers.filter((b) => !b.res).length;
}

export function taskDisplay(t: Task): TaskDisplay {
  if (t.st === "done") return "done";
  if (openBlockers(t) > 0) return "blocked";
  return t.st === "prog" ? "prog" : "ready";
}

export function parentDone(p: Parent): boolean {
  return p.tasks.length > 0 && p.tasks.every((t) => t.st === "done");
}

export function parentDisplay(p: Parent): ParentDisplay {
  if (parentDone(p)) return "done";
  if (p.mode === "hold") return "hold";
  if (p.mode === "todo") return "todo";
  if (p.mode === "prog" || p.tasks.length > 0) return "prog";
  return "todo";
}

export function parentPct(p: Parent): number {
  if (!p.tasks.length) return 0;
  const done = p.tasks.filter((t) => t.st === "done").length;
  return Math.round((done / p.tasks.length) * 100);
}

export function inScope(pr: Project): Parent[] {
  return pr.parents.filter((p) => p.quad !== "q4");
}

export function projectDone(pr: Project): boolean {
  const s = inScope(pr);
  return s.length > 0 && s.every(parentDone);
}

export function daysLeft(iso: string, today: Date = new Date()): number | null {
  if (!iso) return null;
  const t0 = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const d = new Date(iso + "T00:00:00");
  return Math.round((d.getTime() - t0.getTime()) / 86400000);
}

export function dueLabel(iso: string, today: Date = new Date()): string {
  const d = daysLeft(iso, today);
  if (d === null) return "";
  if (d < 0) return "OVERDUE " + -d + "d";
  if (d <= 7) return "DUE IN " + d + "d";
  return "";
}

export function niceDate(iso: string): string {
  if (!iso) return "-";
  return new Date(iso + "T00:00:00").toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

/** Only plain http(s) links are accepted, so a pasted "javascript:" URL can never be stored. */
export function safeUrl(raw: string): string | null {
  const text = raw.trim();
  if (!text) return null;
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? text : "https://" + text;
  try {
    const u = new URL(withScheme);
    if (u.protocol !== "http:" && u.protocol !== "https:") return null;
    return u.href;
  } catch (e) {
    return null;
  }
}
