import { defineStore } from "pinia";
import * as api from "@/api/wiki";
import type { WikiPage } from "@/types/wiki";

// Saves run one at a time so each request carries the version the previous
// one produced; otherwise rapid saves would trip the server's 409 check.
let chain: Promise<unknown> = Promise.resolve();

type State = {
  pages: Record<string, WikiPage[]>;
  loaded: Record<string, boolean>;
};

export const useWikiStore = defineStore("wiki", {
  state: (): State => ({ pages: {}, loaded: {} }),
  getters: {
    forProject: (state) => (projectId: string) => state.pages[projectId] ?? [],
  },
  actions: {
    /** Loads a project's page list. Bodies already fetched are kept while unchanged. */
    async load(projectId: string, force = false) {
      if (this.loaded[projectId] && !force) return;
      const fresh = await api.list(projectId);
      const old = this.pages[projectId] ?? [];
      this.pages[projectId] = fresh.map((p) => {
        const prev = old.find((o) => o.id === p.id);
        return prev && prev.version === p.version && prev.body !== undefined
          ? { ...p, body: prev.body }
          : p;
      });
      this.loaded[projectId] = true;
    },

    /** Fetches one page with its body and merges it into the list. */
    async open(projectId: string, id: string): Promise<WikiPage> {
      const full = await api.get(projectId, id);
      this.upsert(projectId, full);
      return full;
    },

    upsert(projectId: string, page: WikiPage) {
      const list = this.pages[projectId] ?? (this.pages[projectId] = []);
      const i = list.findIndex((p) => p.id === page.id);
      if (i >= 0) list[i] = page;
      else list.push(page);
    },

    /** Saves a page (version 0 creates it) and returns what the server stored. */
    save(projectId: string, page: WikiPage): Promise<WikiPage> {
      const run = chain.then(async () => {
        try {
          const saved = await api.save(projectId, JSON.parse(JSON.stringify(page)));
          this.upsert(projectId, saved);
          return saved;
        } catch (e: any) {
          if (e && e.status === 409) {
            // Someone else saved first: refresh the list so the tree is current.
            await this.load(projectId, true);
          }
          throw e;
        }
      });
      chain = run.catch(() => undefined);
      return run;
    },

    async remove(projectId: string, id: string) {
      await chain;
      await api.remove(projectId, id);
      this.pages[projectId] = (this.pages[projectId] ?? []).filter((p) => p.id !== id);
    },

    /** Drops cached pages for a project, for example after the project is deleted. */
    forget(projectId: string) {
      delete this.pages[projectId];
      delete this.loaded[projectId];
    },
  },
});
