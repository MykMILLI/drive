import { fetchJSON, fetchURL } from "./utils";
import type { WikiPage, WikiRevision } from "@/types/wiki";

const base = (projectId: string) =>
  `/api/projects/${encodeURIComponent(projectId)}/wiki`;

// The server stores pages as opaque JSON, so tolerate documents written by
// other clients that leave optional fields out.
function normalise(p: WikiPage): WikiPage {
  return { ...p, parentId: p.parentId ?? "", refs: p.refs ?? [] };
}

/** Lists a project's pages. Bodies are omitted; use get() for those. */
export async function list(projectId: string): Promise<WikiPage[]> {
  const pages = await fetchJSON<WikiPage[]>(base(projectId));
  return pages.map(normalise);
}

export async function get(projectId: string, id: string): Promise<WikiPage> {
  return normalise(
    await fetchJSON<WikiPage>(`${base(projectId)}/${encodeURIComponent(id)}`)
  );
}

/** Creates (version 0) or updates a page. Returns the saved document. */
export async function save(projectId: string, p: WikiPage): Promise<WikiPage> {
  const res = await fetchURL(`${base(projectId)}/${encodeURIComponent(p.id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(p),
  });
  return normalise((await res.json()) as WikiPage);
}

export async function remove(projectId: string, id: string): Promise<void> {
  await fetchURL(`${base(projectId)}/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function history(
  projectId: string,
  id: string
): Promise<WikiRevision[]> {
  return fetchJSON<WikiRevision[]>(
    `${base(projectId)}/${encodeURIComponent(id)}/history`
  );
}

export async function revision(
  projectId: string,
  id: string,
  version: number
): Promise<WikiPage> {
  return normalise(
    await fetchJSON<WikiPage>(
      `${base(projectId)}/${encodeURIComponent(id)}/history/${version}`
    )
  );
}
