import { defineStore } from "pinia";
import { drive as api } from "@/api";

/** Normalises a scope-relative path the same way the backend does. */
export function normalizeStarPath(p: string): string {
  let out = "/" + p.trim().replace(/^\/+/, "");
  if (out.length > 1) out = out.replace(/\/+$/, "");
  return out;
}

export const useStarredStore = defineStore("starred", {
  state: (): { paths: Set<string>; loaded: boolean } => ({
    paths: new Set<string>(),
    loaded: false,
  }),
  actions: {
    isStarred(p: string) {
      return this.paths.has(normalizeStarPath(p));
    },
    async load(force = false) {
      if (this.loaded && !force) return;
      const items = await api.starred();
      this.paths = new Set(items.map((i) => normalizeStarPath(i.path)));
      this.loaded = true;
      return items;
    },
    async toggle(p: string) {
      const path = normalizeStarPath(p);
      const next = !this.paths.has(path);
      const list = await api.setStarred(path, next);
      this.paths = new Set(list.map(normalizeStarPath));
      return next;
    },
  },
});
