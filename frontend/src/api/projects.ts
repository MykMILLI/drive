import { fetchJSON, fetchURL } from "./utils";
import type { Project } from "@/types/projects";

export async function list(): Promise<Project[]> {
  return fetchJSON<Project[]>("/api/projects");
}

/** Creates (version 0) or updates a project. Returns the saved document. */
export async function save(p: Project): Promise<Project> {
  const res = await fetchURL(`/api/projects/${encodeURIComponent(p.id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(p),
  });
  return (await res.json()) as Project;
}

export async function remove(id: string): Promise<void> {
  await fetchURL(`/api/projects/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}
