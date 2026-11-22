import { fetchJSON, fetchURL } from "./utils";

export interface RecentResponse {
  items: RecursiveEntry[];
  partial: boolean;
}

export interface SlugCheck {
  slug: string;
  available: boolean;
  error?: string;
}

export async function starred() {
  return fetchJSON<RecursiveEntry[]>("/api/starred");
}

export async function setStarred(path: string, starred: boolean) {
  const res = await fetchURL("/api/starred", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, starred }),
  });
  return (await res.json()) as string[];
}

export async function recent(limit = 50) {
  return fetchJSON<RecentResponse>(`/api/recent?limit=${limit}`);
}

export async function checkSlug(slug: string) {
  return fetchJSON<SlugCheck>(`/api/share-slug/${encodeURIComponent(slug)}`);
}

export async function sendMagicLink(email: string) {
  await fetchURL(
    "/api/auth/magic-link",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
    },
    false
  );
}
