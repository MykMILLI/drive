<template>
  <div class="proj-root">
    <header-bar showMenu />

    <div class="page-head">
      <div>
        <h1>Projects</h1>
        <p class="muted">Tickets and work items as projects: matrix, tasks, roadblocks.</p>
      </div>
      <div class="pj-actions">
        <button class="pb pri" type="button" @click="showImport = true">Import ticket</button>
        <button class="pb" type="button" @click="newProject">New project</button>
      </div>
    </div>

    <div v-if="loading" class="empty-state">
      <div class="spinner"><div class="bounce1"></div><div class="bounce2"></div><div class="bounce3"></div></div>
    </div>

    <template v-else>
      <div class="tiles">
        <div class="tile"><b>{{ store.items.length }}</b><span>Projects</span></div>
        <div class="tile"><b>{{ doneParents }}/{{ scoped.length }}</b><span>Parents done</span></div>
        <div class="tile"><b>{{ doneTasks }}/{{ tasks.length }}</b><span>Tasks done</span></div>
        <div class="tile" style="color: var(--pj-amber)"><b>{{ progTasks }}</b><span>In progress</span></div>
        <div class="tile" style="color: var(--pj-red)"><b>{{ blockedTasks.length }}</b><span>Blocked tasks</span></div>
      </div>

      <h2 class="pj-h">Projects</h2>
      <div v-if="!store.items.length" class="empty-state">
        <h3>No projects yet</h3>
        <p>Import a hub ticket or start a blank project.</p>
      </div>
      <div class="cards-grid">
        <div v-for="pr in store.items" :key="pr.id" class="pcard" @click="openProject(pr.id)">
          <h3>{{ pr.name }}</h3>
          <div v-if="pr.ticket">
            <span class="tk">TICKET #{{ pr.ticket.id }}</span>
            <span v-if="pr.ticket.priority" class="badge s-todo">{{ pr.ticket.priority.toUpperCase() }}</span>
            <span v-if="!projectDone(pr) && dueLabel(pr.ticket.due)" class="badge s-blocked">{{ dueLabel(pr.ticket.due) }}</span>
          </div>
          <div class="bar"><i :style="{ width: progress(pr) + '%' }"></i></div>
          <div class="muted">
            {{ inScope(pr).filter(parentDone).length }} of {{ inScope(pr).length }} parents done
            <span v-if="blockedIn(pr)" style="color: var(--pj-red)"> · {{ blockedIn(pr) }} blocked</span>
            <span v-if="projectDone(pr)"> · complete</span>
          </div>
        </div>
      </div>

      <h2 class="pj-h">All parent items - drag between quadrants</h2>
      <div class="matrix">
        <div
          v-for="q in QUADS"
          :key="q.key"
          class="quad"
          :class="[q.key, { over: overQuad === q.key }]"
          @dragover.prevent="overQuad = q.key"
          @dragleave="overQuad = ''"
          @drop.prevent="dropOnQuad(q.key)"
        >
          <h3>{{ q.name }}</h3>
          <small>{{ q.hint }}</small>
          <div class="cards">
            <div
              v-for="x in items.filter((i) => i.p.quad === q.key)"
              :key="x.p.id"
              class="pcard-mini"
              :class="q.key"
              draggable="true"
              @dragstart="dragId = x.p.id"
              @dragend="clearDrag"
              @click="openProject(x.pr.id, x.p.id)"
            >
              <div class="pjn">{{ x.pr.name.toUpperCase() }}</div>
              <div class="t"><span>{{ x.p.name }}</span><span class="muted">{{ parentPct(x.p) }}%</span></div>
              <div class="bar"><i :style="{ width: parentPct(x.p) + '%' }"></i></div>
              <div class="m">
                <span class="badge" :class="'s-' + parentDisplay(x.p)">{{ label(parentDisplay(x.p)) }}</span>
                <span>{{ x.p.tasks.filter((t) => t.st === "done").length }}/{{ x.p.tasks.length }} tasks</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <h2 class="pj-h">Needs attention - open roadblocks</h2>
      <div class="list">
        <div v-for="(r, i) in roadblocks" :key="i" class="li" @click="openProject(r.pr.id, r.p.id)">
          <span class="badge s-blocked">BLOCKED</span>
          <div class="g"><b>{{ r.t.text }}</b><div class="muted">{{ r.pr.name }} › {{ r.p.name }}</div></div>
          <div>{{ r.b.text }}</div>
          <div class="muted">{{ r.b.owner || "no owner" }}</div>
        </div>
        <div v-if="!roadblocks.length" class="li"><span class="ok">Nothing blocked</span></div>
      </div>

      <h2 class="pj-h">Coming due</h2>
      <div class="list">
        <div v-for="x in due" :key="x.t.id" class="li" @click="openProject(x.pr.id, x.p.id)">
          <span class="badge" :class="'s-' + taskDisplay(x.t)">{{ label(taskDisplay(x.t)) }}</span>
          <div class="g"><b>{{ x.t.text }}</b><div class="muted">{{ x.pr.name }} › {{ x.p.name }}</div></div>
          <div>{{ x.t.date }} <span v-if="dueLabel(x.t.date)" class="badge s-blocked">{{ dueLabel(x.t.date) }}</span></div>
        </div>
        <div v-if="!due.length" class="li muted">No dated tasks</div>
      </div>
    </template>

    <project-import v-if="showImport" @close="showImport = false" @imported="onImported" />
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import HeaderBar from "@/components/header/HeaderBar.vue";
import ProjectImport from "@/components/projects/ProjectImport.vue";
import { useProjectsStore } from "@/stores/projects";
import { QUADS } from "@/utils/quadrants";
import {
  dueLabel,
  inScope,
  parentDisplay,
  parentDone,
  parentPct,
  projectDone,
  taskDisplay,
} from "@/utils/projectLogic";
import { slugify, uniqueId } from "@/utils/ticketImport";
import type { Project, Quad } from "@/types/projects";
import "@/css/projects.css";

const $showError = inject<IToastError>("$showError")!;
const router = useRouter();
const store = useProjectsStore();
const loading = ref(true);
const showImport = ref(false);
const overQuad = ref("");
const dragId = ref("");

const LABELS: Record<string, string> = {
  todo: "NOT STARTED",
  ready: "READY",
  prog: "IN PROGRESS",
  blocked: "BLOCKED",
  done: "DONE",
  hold: "ON HOLD",
};
const label = (k: string) => LABELS[k] || k;

const items = computed(() =>
  store.items.flatMap((pr) => pr.parents.map((p) => ({ p, pr })))
);
const scoped = computed(() => items.value.filter((x) => x.p.quad !== "q4"));
const tasks = computed(() =>
  items.value.flatMap(({ p, pr }) => p.tasks.map((t) => ({ t, p, pr })))
);
const doneParents = computed(() => scoped.value.filter((x) => parentDone(x.p)).length);
const doneTasks = computed(() => tasks.value.filter((x) => x.t.st === "done").length);
const progTasks = computed(() => tasks.value.filter((x) => taskDisplay(x.t) === "prog").length);
const blockedTasks = computed(() =>
  tasks.value.filter((x) => x.p.quad !== "q4" && taskDisplay(x.t) === "blocked")
);
const roadblocks = computed(() =>
  blockedTasks.value.flatMap((x) =>
    x.t.blockers.filter((b) => !b.res).map((b) => ({ ...x, b }))
  )
);
const due = computed(() =>
  tasks.value
    .filter((x) => x.t.st !== "done" && x.t.date && x.p.quad !== "q4")
    .sort((a, b) => a.t.date.localeCompare(b.t.date))
    .slice(0, 6)
);

const progress = (pr: Project) => {
  const s = inScope(pr);
  return s.length ? (s.filter(parentDone).length / s.length) * 100 : 0;
};
const blockedIn = (pr: Project) =>
  pr.parents.flatMap((p) => p.tasks).filter((t) => taskDisplay(t) === "blocked").length;

function openProject(id: string, sel?: string) {
  router.push({ path: `/projects/${id}`, query: sel ? { sel } : {} });
}

async function newProject() {
  const id = uniqueId(slugify("new project"), store.takenIds);
  const p: Project = { id, name: "New project", version: 0, parents: [], links: [], cm: [] };
  try {
    await store.create(p);
    openProject(id);
  } catch (e: any) {
    $showError(e);
  }
}

function onImported(id: string) {
  showImport.value = false;
  openProject(id);
}

function clearDrag() {
  dragId.value = "";
  overQuad.value = "";
}

async function dropOnQuad(q: Quad) {
  const hit = items.value.find((x) => x.p.id === dragId.value);
  clearDrag();
  if (!hit || hit.p.quad === q) return;
  hit.p.quad = q;
  try {
    await store.persist(hit.pr.id);
  } catch (e: any) {
    $showError(e);
  }
}

onMounted(async () => {
  try {
    await store.load(true);
  } catch (e: any) {
    $showError(e);
  } finally {
    loading.value = false;
  }
});
</script>
