import type { Quad } from "@/types/projects";

export const QUADS: { key: Quad; name: string; hint: string }[] = [
  { key: "q1", name: "Do first", hint: "Urgent + important - handle now" },
  { key: "q2", name: "Schedule", hint: "Important, not urgent - put it on the calendar" },
  { key: "q3", name: "Limit", hint: "Urgent, not important - time-box, batch, or automate" },
  { key: "q4", name: "Eliminate", hint: "Neither - dropped from project scope" },
];
