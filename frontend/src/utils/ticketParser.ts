import type { ParsedTicket, TicketEntry } from "@/types/projects";

const HEADER =
  /^(.+?)\s·\s(\d{1,2}\/\d{1,2}\/\d{4},?\s*\d{1,2}:\d{2}\s*[AP]M)$/i;
const ACTIVITY =
  /(created work order|link shared|changed|assigned|status|marked|started|revoked)/i;
const PRIORITY_LINE =
  /^(?:(None|Low|Medium|High))?\s*(Contractor link)?\s*(Revoke)?$/i;

function iso(s: string | undefined): string {
  const m = s ? s.match(/(\d{1,2})\/(\d{1,2})\/(\d{4})/) : null;
  return m ? `${m[3]}-${m[1].padStart(2, "0")}-${m[2].padStart(2, "0")}` : "";
}

/**
 * Parses text copied from a hub contractor ticket (select-all + copy, or the
 * bookmarklet's document.body.innerText). Returns null when no "#1234 · CATEGORY"
 * header is present. Tolerates the header/priority/status chips being joined on
 * one line or split across several.
 */
export function parseTicket(raw: string): ParsedTicket | null {
  let L = raw
    .replace(/\r/g, "")
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
  const start = L.findIndex((l) => /^#\d+\s*·/.test(l));
  if (start < 0) return null;
  L = L.slice(start);

  const head = L[0].match(/^#(\d+)\s*·\s*(.+)$/);
  if (!head) return null;
  const t: ParsedTicket = {
    id: head[1],
    category: head[2].trim(),
    title: L[1] || "Untitled ticket",
    priority: "",
    kind: "",
    due: "",
    created: "",
    items: [],
    entries: [],
    assignee: "",
    desc: "",
    linkExpires: 0,
    warn: [],
  };

  let i = 2;
  while (i < L.length && PRIORITY_LINE.test(L[i])) {
    const g = PRIORITY_LINE.exec(L[i]);
    if (g && g[1] && !t.priority) t.priority = g[1];
    i++;
  }

  const j = L.findIndex(
    (l, k) => k >= i && /due\s+\d{1,2}\/\d{1,2}\/\d{4}/i.test(l)
  );
  if (j >= 0) {
    t.desc = L.slice(i, j).join("\n");
    const meta = L.slice(j, j + 4).join(" ");
    const kind = L[j].match(/^([A-Za-z ]+?)\s*·/);
    t.kind = kind ? kind[1] : "";
    const due = meta.match(/due\s+(\d{1,2}\/\d{1,2}\/\d{4})/i);
    const created = meta.match(/created\s+(\d{1,2}\/\d{1,2}\/\d{4})/i);
    t.due = iso(due ? due[1] : undefined);
    t.created = iso(created ? created[1] : undefined);
  } else {
    t.warn.push("No due date found");
  }

  const c = L.findIndex((l) => /^Checklist$/i.test(l));
  if (c >= 0) {
    for (let k = c + 1; k < L.length; k++) {
      const l = L[k];
      if (/^\d+\s+of\s+\d+\s+done$/i.test(l)) continue;
      if (/^(Add|Edit steps.*|Comments and activity)$/i.test(l)) break;
      t.items.push(l);
    }
  } else {
    t.warn.push("No checklist found");
  }

  const s = L.findIndex((l) => /^Comments and activity$/i.test(l));
  const e = L.findIndex((l, k) => k > s && /^Photos$/i.test(l));
  if (s >= 0) {
    let cur: TicketEntry | null = null;
    for (const l of L.slice(s + 1, e < 0 ? L.length : e)) {
      if (/^(Send|\d+|Edit)$/i.test(l)) continue;
      const h = HEADER.exec(l);
      if (h) {
        const at = new Date(h[2].replace(",", "")).getTime() || Date.now();
        const act = ACTIVITY.test(h[1]);
        cur = { by: act ? "" : h[1], at, text: act ? h[1] : "", activity: act };
        t.entries.push(cur);
        const lk = h[1].match(/good for (\d+) days/i);
        if (lk) t.linkExpires = at + Number(lk[1]) * 86400000;
      } else if (cur) {
        cur.text += (cur.text ? "\n" : "") + l;
      }
    }
  }

  const a = L.findIndex((l) => /^Assigned to$/i.test(l));
  if (a >= 0) {
    for (let k = a + 1; k < L.length && !/^(Priority|Watchers)/i.test(L[k]); k++) {
      const n = L[k].match(/^(.+?)\s*×$/);
      if (n && !t.assignee) t.assignee = n[1];
    }
  }
  return t;
}
