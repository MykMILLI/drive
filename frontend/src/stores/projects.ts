import { defineStore } from "pinia";
import * as api from "@/api/projects";
import type { Project } from "@/types/projects";

// Saves run one at a time so each request carries the version the previous
// one produced. Without this, rapid edits would trip the 409 version check.
let chain: Promise<unknown> = Promise.resolve();

export const useProjectsStore = defineStore("projects", {
  state: (): { items: Project[]; loaded: boolean } => ({
    items: [],
    loaded: false,
  }),
  getters: {
    byId: (state) => (id: string) => state.items.find((p) => p.id === id),
    takenIds: (state) => state.items.map((p) => p.id),
  },
  actions: {
    async load(force = false) {
      if (this.loaded && !force) return;
      this.items = await api.list();
      this.loaded = true;
    },

    /** Adds a brand-new project locally and saves it. */
    async create(p: Project) {
      this.items.push(p);
      await this.persist(p.id);
    },

    /** Saves the current in-memory state of a project. */
    persist(id: string): Promise<void> {
      const run = chain.then(async () => {
        const p = this.items.find((x) => x.id === id);
        if (!p) return;
        try {
          const saved = await api.save(JSON.parse(JSON.stringify(p)));
          p.version = saved.version;
          p.updatedAt = saved.updatedAt;
        } catch (e: any) {
          if (e && e.status === 409) {
            // Someone else saved first: reload so the screen matches disk.
            await this.load(true);
          }
          throw e;
        }
      });
      chain = run.catch(() => undefined);
      return run;
    },

    async remove(id: string) {
      await chain;
      await api.remove(id);
      this.items = this.items.filter((p) => p.id !== id);
    },
  },
});
