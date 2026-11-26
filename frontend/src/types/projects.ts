export type Quad = "q1" | "q2" | "q3" | "q4";
export type TaskState = "todo" | "prog" | "done";
export type TaskDisplay = "todo" | "ready" | "prog" | "blocked" | "done";
export type ParentMode = "auto" | "todo" | "prog" | "hold";
export type ParentDisplay = "todo" | "prog" | "done" | "hold";

export interface Blocker {
  text: string;
  owner: string;
  res: boolean;
}

export interface ProjectComment {
  id: string;
  by: string;
  at: number;
  text: string;
  activity?: boolean;
  key?: string;
}

export interface ProjectLink {
  id: string;
  name: string;
  url: string;
  at: number;
}

export interface Task {
  id: string;
  text: string;
  date: string;
  st: TaskState;
  blockers: Blocker[];
  links: ProjectLink[];
  cm: ProjectComment[];
}

export interface Parent {
  id: string;
  name: string;
  quad: Quad;
  owner: string;
  target: string;
  mode: ParentMode;
  tasks: Task[];
  links: ProjectLink[];
  cm: ProjectComment[];
  fromTicket?: boolean;
}

export interface TicketInfo {
  id: string;
  category: string;
  priority: string;
  kind: string;
  due: string;
  created: string;
  assignee: string;
  desc: string;
  linkExpires: number;
  importedAt: number;
  url: string;
}

export interface Project {
  id: string;
  name: string;
  version: number;
  updatedAt?: string;
  parents: Parent[];
  links: ProjectLink[];
  cm: ProjectComment[];
  ticket?: TicketInfo;
}

export interface TicketEntry {
  by: string;
  at: number;
  text: string;
  activity: boolean;
}

export interface ParsedTicket {
  id: string;
  category: string;
  title: string;
  priority: string;
  kind: string;
  due: string;
  created: string;
  items: string[];
  entries: TicketEntry[];
  assignee: string;
  desc: string;
  linkExpires: number;
  warn: string[];
}

export interface ImportResult {
  project: Project;
  isNew: boolean;
  quad: Quad;
  added: { tasks: number; comments: number; roadblocks: number };
}
