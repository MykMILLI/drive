import { describe, expect, it } from "vitest";
import { parseTicket } from "./ticketParser";
import { importTicket, slugify } from "./ticketImport";
import {
  dueLabel,
  parentDisplay,
  projectDone,
  safeUrl,
  taskDisplay,
} from "./projectLogic";

// Real hub ticket #1552, copied with select-all (chips run together on one line).
const TICKET = `#1552 · GENERAL
Migrate Standalone Microsoft Tenants to Main Tenant
MediumContractor linkRevoke
Migrate all standalone Microsoft tenants across all offices under the main Cleveland Dental Management tenant.
Standalone tenants mean different accounts we have to keep up with, different pricing for licenses, and multiple billing dates to manage. I will streamline this across all offices so we only need to look in one place for M365 management.
Reactive · due 11/12/2026 · created 10/16/2026 · 0 of 5 done
Start work
Starts the clock, puts you on the job, and marks it In Progress. Everything saves on its own.
OpenOn HoldIn ProgressDone
Checklist
0 of 5 done
Inventory all tenants to confirm which ones need migrated
Confirm admin access into all standalone tenants to ensure migration is possible
Purchase BitTitan license if necessary to migrate emails from standalone tenant to CDM tenant
Migrate Tenants
Ensure licenses from standalone tenants are transferred over to the CDM so accounts stay active
Add
Edit stepsCopy steps from another work order
Comments and activity
2
Send
Mykal Mills contractor link shared, good for 30 days · 11/26/2026, 8:16 AM
Mykal Mills · 11/4/2026, 10:12 AM
Edit
I now have access to Tomorrow's Dentistry's M365 environment
Mykal Mills · 10/26/2026, 9:44 AM
Edit
Five Star, Tomorrow's Dentistry and Aurora all have standalone M365 licenses we are paying for outside our main tenant. Working to get direct access to Aurora and Tomorrow's Dentistry.
Mykal Mills created work order · 10/16/2026, 11:24 AM
Photos
Add photos
No photos yet.
Assigned to
+ Assign
Mykal Mills×
PriorityNoneLowMediumHigh
Watchers hear when the status changes and when it is done. They are not assigned the work.`;

const TODAY = new Date(2026, 10, 26); // Nov 26, 2026

describe("parseTicket", () => {
  it("reads the header, priority, dates, checklist, comments and assignee", () => {
    const t = parseTicket(TICKET)!;
    expect(t.id).toBe("1552");
    expect(t.category).toBe("GENERAL");
    expect(t.title).toBe("Migrate Standalone Microsoft Tenants to Main Tenant");
    expect(t.priority).toBe("Medium");
    expect(t.kind).toBe("Reactive");
    expect(t.due).toBe("2026-11-12");
    expect(t.created).toBe("2026-10-16");
    expect(t.items).toHaveLength(5);
    expect(t.assignee).toBe("Mykal Mills");
    expect(t.entries).toHaveLength(4);
    expect(t.entries.filter((e) => e.activity)).toHaveLength(2);
    expect(t.linkExpires).toBeGreaterThan(0);
    expect(t.desc.startsWith("Migrate all standalone")).toBe(true);
  });

  it("handles chips split across lines and surrounding page text", () => {
    const split = TICKET.replace("MediumContractor linkRevoke", "Medium\nContractor link\nRevoke");
    expect(parseTicket("Home\nTickets\n" + split + "\nfooter")!.items).toHaveLength(5);
  });

  it("returns null when there is no ticket header", () => {
    expect(parseTicket("hello world")).toBeNull();
  });
});

describe("importTicket", () => {
  it("creates a project, places it in Do first, and suggests roadblocks", () => {
    const t = parseTicket(TICKET)!;
    const r = importTicket(t, undefined, [], TODAY);
    expect(r.isNew).toBe(true);
    expect(r.quad).toBe("q1");
    expect(r.project.id).toMatch(/^[a-z0-9][a-z0-9-]{0,63}$/);
    expect(r.added).toEqual({ tasks: 5, comments: 4, roadblocks: 2 });
    expect(r.project.parents[0].tasks.filter((x) => taskDisplay(x) === "blocked")).toHaveLength(2);
    expect(dueLabel(t.due, TODAY)).toBe("OVERDUE 14d");
  });

  it("does not duplicate anything when the same ticket is re-imported", () => {
    const t = parseTicket(TICKET)!;
    const first = importTicket(t, undefined, [], TODAY);
    const again = importTicket(t, first.project, [first.project.id], TODAY);
    expect(again.isNew).toBe(false);
    expect(again.added).toEqual({ tasks: 0, comments: 0, roadblocks: 0 });
  });

  it("closes the parent and project once every task is done", () => {
    const r = importTicket(parseTicket(TICKET)!, undefined, [], TODAY);
    const parent = r.project.parents[0];
    expect(parentDisplay(parent)).toBe("prog");
    parent.tasks.forEach((x) => {
      x.blockers.forEach((b) => (b.res = true));
      x.st = "done";
    });
    expect(projectDone(r.project)).toBe(true);
  });
});

describe("helpers", () => {
  it("slugifies and keeps ids Drive-safe", () => {
    expect(slugify("  Hello, World!! ")).toBe("hello-world");
  });
  it("rejects non-http links", () => {
    expect(safeUrl("javascript:alert(1)")).toBeNull();
    expect(safeUrl("example.com/a")).toBe("https://example.com/a");
  });
});
