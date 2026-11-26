<template>
  <div class="proj-root">
    <header-bar showMenu />

    <div v-if="loading" class="empty-state">
      <div class="spinner"><div class="bounce1"></div><div class="bounce2"></div><div class="bounce3"></div></div>
    </div>

    <div v-else-if="!project" class="empty-state">
      <h3>Project not found</h3>
      <router-link to="/projects">Back to projects</router-link>
    </div>

    <template v-else>
      <div class="page-head">
        <div>
          <router-link to="/projects" class="back">← Projects</router-link>
          <div><input v-model="project.name" class="title-input" aria-label="Project name" @change="persist" /></div>
          <div class="muted">
            {{ doneCount }} of {{ scoped.length }} parent items complete
          </div>
        </div>
        <div class="pj-actions">
          <button class="pb" type="button" @click="openDrawer('project', project.id)">
            📎 {{ project.links.length }} · 💬 {{ project.cm.length }}
          </button>
          <router-link class="pb" :to="folder">Open folder</router-link>
          <button class="pb" :class="{ danger: armDelete }" type="button" @click="deleteProject">
            {{ armDelete ? "Click again to confirm" : "Delete project" }}
          </button>
        </div>
      </div>
      <div class="bar" style="margin-bottom: 0.8em"><i :style="{ width: (scoped.length ? (doneCount / scoped.length) * 100 : 0) + '%' }"></i></div>
      <div v-if="scoped.length && doneCount === scoped.length" class="celebrate">Project complete - every in-scope parent item is closed.</div>

      <div v-if="project.ticket" class="tkp">
        <div class="r">
          <span class="tk">TICKET #{{ project.ticket.id }}</span>
          <b>{{ project.ticket.category }}</b>
          <span v-if="project.ticket.priority" class="badge s-todo">{{ project.ticket.priority.toUpperCase() }}</span>
          <span v-if="project.ticket.kind">{{ project.ticket.kind }}</span>
          <span>Due <b>{{ niceDate(project.ticket.due) }}</b></span>
          <span v-if="dueLabel(project.ticket.due) && !projectDone(project)" class="badge s-blocked">{{ dueLabel(project.ticket.due) }}</span>
          <span>Created {{ niceDate(project.ticket.created) }}</span>
          <span v-if="project.ticket.assignee">Assigned <b>{{ project.ticket.assignee }}</b></span>
          <span v-if="linkDays !== null" :class="{ warn: linkDays <= 7 }">
            {{ linkDays > 0 ? "Hub link expires in " + linkDays + "d" : "Hub link expired - everything is saved here" }}
          </span>
          <button class="pb" style="margin-left: auto" type="button" @click="showImport = true">Re-import / update</button>
        </div>
        <div v-if="project.ticket.desc" class="d">{{ project.ticket.desc }}</div>
        <div class="r">
          <span>Hub link</span>
          <input v-model="project.ticket.url" placeholder="Paste the hub ticket URL (optional)" @change="persist" />
          <a v-if="safeLink" :href="safeLink" target="_blank" rel="noopener noreferrer">Open</a>
        </div>
      </div>

      <h2 class="pj-h">1 · Priority matrix - parent items</h2>
      <div class="matrix">
        <div
          v-for="q in QUADS"
          :key="q.key"
          class="quad"
          :class="[q.key, { over: overQuad === q.key }]"
          @dragover="onQuadOver($event, q.key)"
          @dragleave="overQuad = ''"
          @drop="dropOnQuad($event, q.key)"
        >
          <h3>{{ q.name }}</h3>
          <small>{{ q.hint }}</small>
          <div class="cards">
            <div
              v-for="p in project.parents.filter((x) => x.quad === q.key)"
              :key="p.id"
              class="pcard-mini"
              :class="[q.key, { sel: p.id === selId, taskover: overCard === p.id }]"
              draggable="true"
              @dragstart="startParentDrag($event, p.id)"
              @dragend="clearDrag"
              @dragover="onCardOver($event, p.id)"
              @dragleave="overCard = ''"
              @drop="dropOnCard($event, p.id)"
              @click="selId = p.id"
            >
              <div class="t"><span>{{ p.name }}</span><span class="muted">{{ parentPct(p) }}%</span></div>
              <div class="bar"><i :style="{ width: parentPct(p) + '%' }"></i></div>
              <div class="m">
                <span class="badge" :class="'s-' + parentDisplay(p)">{{ label(parentDisplay(p)) }}</span>
                <span>{{ p.tasks.filter((t) => t.st === "done").length }}/{{ p.tasks.length }} tasks</span>
                <span v-if="blockedIn(p)" style="color: var(--pj-red)">{{ blockedIn(p) }} blocked</span>
              </div>
            </div>
          </div>
          <button class="pb dashed" style="width: 100%; margin-top: 0.6em" type="button" @click="addParent(q.key)">+ Add parent item</button>
        </div>
      </div>

      <h2 class="pj-h">2 · Bracket - selected parent</h2>
      <div v-if="!sel" class="muted">Select or add a parent item.</div>
      <div v-else class="detail" :class="sel.quad">
        <div class="dhead">
          <input v-model="sel.name" class="nm" aria-label="Parent name" @change="persist" />
          <span class="badge" :class="'s-' + parentDisplay(sel)">{{ label(parentDisplay(sel)) }}</span>
          <button class="pb" type="button" @click="openDrawer('parent', sel.id)">📎 {{ sel.links.length }} · 💬 {{ sel.cm.length }}</button>
          <button class="pb" type="button" @click="deleteParent">Delete parent</button>
        </div>
        <div class="meta">
          Priority
          <select v-model="sel.quad" @change="persist">
            <option v-for="q in QUADS" :key="q.key" :value="q.key">{{ q.name }}</option>
          </select>
          Status
          <select v-model="sel.mode" @change="persist">
            <option value="auto">Auto ({{ label(parentDisplay(sel)).toLowerCase() }})</option>
            <option value="todo">Not started</option>
            <option value="prog">In progress</option>
            <option value="hold">On hold</option>
          </select>
          Owner <input v-model="sel.owner" style="width: 120px" @change="persist" />
          Target <input v-model="sel.target" type="date" @change="persist" />
        </div>

        <div class="cols"><div>Action items</div><div>Due</div><div>Roadblocks / unknowns</div></div>
        <div v-if="!sel.tasks.length" class="muted" style="margin: 0.6em 0">No action items yet - add one to start this parent.</div>

        <div v-for="t in sel.tasks" :key="t.id" class="row" :class="{ dragging: dragTask === t.id }">
          <div class="task" :class="[taskDisplay(t), { shake: shakeId === t.id }]">
            <span class="grip" draggable="true" title="Drag onto another parent card" @dragstart.stop="startTaskDrag($event, t.id)" @dragend="clearDrag">⠿</span>
            <input v-model="t.text" class="tx" aria-label="Task" @change="persist" />
            <select class="tst" :class="'s-' + taskDisplay(t)" :value="t.st" aria-label="Task status" @change="setStatus(t, $event)">
              <option value="todo">{{ taskDisplay(t) === "blocked" ? "Blocked" : taskDisplay(t) === "ready" ? "Ready" : "Not started" }}</option>
              <option value="prog">In progress</option>
              <option value="done">Done</option>
            </select>
            <button class="pb" type="button" @click="openDrawer('task', t.id)">📎 {{ t.links.length }} · 💬 {{ t.cm.length }}</button>
            <button class="x" type="button" aria-label="Delete task" @click="deleteTask(t.id)">✕</button>
          </div>
          <div><input v-model="t.date" type="date" style="width: 100%" aria-label="Due date" @change="persist" /></div>
          <div>
            <div class="bl">
              <div v-for="(b, i) in t.blockers" :key="i" class="blk" :class="{ res: b.res }">
                <div class="box">
                  <input v-model="b.res" type="checkbox" title="Mark resolved" @change="persist" />
                  <input v-model="b.text" class="bt" aria-label="Roadblock" @change="persist" />
                  <input v-model="b.owner" class="ow" placeholder="owner" @change="persist" />
                  <button class="x" type="button" aria-label="Delete roadblock" @click="deleteBlocker(t, i)">✕</button>
                </div>
              </div>
              <div v-if="!t.blockers.length" class="clear">No roadblocks</div>
            </div>
            <button class="pb addb" type="button" @click="addBlocker(t)">+ Add roadblock / unknown</button>
          </div>
        </div>
        <button class="pb" style="margin-top: 0.4em" type="button" @click="addTask">+ Add action item</button>
      </div>

      <p class="muted" style="margin-top: 1.5em">
        Clear every roadblock, start the task, finish it, close the parent, close the project. Drag a parent card
        to another quadrant. Drag a task by its handle onto another parent card to move it.
      </p>
    </template>

    <item-drawer
      v-if="project && drawerCtx"
      :title="drawerCtx.title"
      :sub="drawerCtx.sub"
      :links="drawerCtx.item.links"
      :cm="drawerCtx.item.cm"
      :folder="folder"
      @close="drawer = null"
      @add-link="onAddLink"
      @remove-link="onRemoveLink"
      @add-comment="onAddComment"
      @remove-comment="onRemoveComment"
    />
    <project-import v-if="showImport" @close="showImport = false" @imported="showImport = false" />
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import HeaderBar from "@/components/header/HeaderBar.vue";
import ItemDrawer from "@/components/projects/ItemDrawer.vue";
import ProjectImport from "@/components/projects/ProjectImport.vue";
import { useProjectsStore } from "@/stores/projects";
import { useAuthStore } from "@/stores/auth";
import { QUADS } from "@/utils/quadrants";
import {
  dueLabel,
  inScope,
  niceDate,
  openBlockers,
  parentDisplay,
  parentDone,
  parentPct,
  projectDone,
  safeUrl,
  taskDisplay,
} from "@/utils/projectLogic";
import { newTask, uid } from "@/utils/ticketImport";
import type { Blocker, Parent, Quad, Task, TaskState } from "@/types/projects";
import "@/css/projects.css";

const $showError = inject<IToastError>("$showError")!;
const route = useRoute();
const router = useRouter();
const store = useProjectsStore();
const auth = useAuthStore();

const loading = ref(true);
const selId = ref("");
const showImport = ref(false);
const armDelete = ref(false);
const shakeId = ref("");
const overQuad = ref("");
const overCard = ref("");
const dragTask = ref("");
const dragParent = ref("");
const drawer = ref<{ kind: "project" | "parent" | "task"; id: string } | null>(null);

const LABELS: Record<string, string> = {
  todo: "NOT STARTED",
  ready: "READY",
  prog: "IN PROGRESS",
  blocked: "BLOCKED",
  done: "DONE",
  hold: "ON HOLD",
};
const label = (k: string) => LABELS[k] || k;

const project = computed(() => store.byId(String(route.params.id)));
const sel = computed(() => project.value?.parents.find((p) => p.id === selId.value));
const scoped = computed(() => (project.value ? inScope(project.value) : []));
const doneCount = computed(() => scoped.value.filter(parentDone).length);
const folder = computed(() => `/files/Projects/${project.value ? project.value.id : ""}/`);
const safeLink = computed(() => (project.value?.ticket?.url ? safeUrl(project.value.ticket.url) : null));
const linkDays = computed(() => {
  const exp = project.value?.ticket?.linkExpires;
  return exp ? Math.ceil((exp - Date.now()) / 86400000) : null;
});
const blockedIn = (p: Parent) => p.tasks.filter((t) => taskDisplay(t) === "blocked").length;

const drawerCtx = computed(() => {
  const pr = project.value;
  const d = drawer.value;
  if (!pr || !d) return null;
  if (d.kind === "project") return { item: pr, title: pr.name, sub: "Project" };
  for (const p of pr.parents) {
    if (d.kind === "parent" && p.id === d.id) return { item: p, title: p.name, sub: "Parent item" };
    const t = p.tasks.find((x) => x.id === d.id);
    if (d.kind === "task" && t) return { item: t, title: t.text, sub: "Task · " + p.name };
  }
  return null;
});

async function persist() {
  if (!project.value) return;
  try {
    await store.persist(project.value.id);
  } catch (e: any) {
    $showError(e);
  }
}

function openDrawer(kind: "project" | "parent" | "task", id: string) {
  drawer.value = { kind, id };
}

function addParent(q: Quad) {
  if (!project.value) return;
  const p: Parent = {
    id: uid(), name: "New parent item", quad: q, owner: "", target: "",
    mode: "auto", tasks: [], links: [], cm: [],
  };
  project.value.parents.push(p);
  selId.value = p.id;
  persist();
}

function deleteParent() {
  if (!project.value || !sel.value) return;
  const gone = sel.value.id;
  project.value.parents = project.value.parents.filter((p) => p.id !== gone);
  selId.value = project.value.parents.length ? project.value.parents[0].id : "";
  persist();
}

function addTask() {
  if (!sel.value) return;
  sel.value.tasks.push(newTask("New action item", "", "todo", []));
  persist();
}
function deleteTask(id: string) {
  if (!sel.value) return;
  sel.value.tasks = sel.value.tasks.filter((t) => t.id !== id);
  persist();
}
function addBlocker(t: Task) {
  const b: Blocker = { text: "New roadblock", owner: "", res: false };
  t.blockers.push(b);
  persist();
}
function deleteBlocker(t: Task, i: number) {
  t.blockers.splice(i, 1);
  persist();
}

function setStatus(t: Task, ev: Event) {
  const el = ev.target as HTMLSelectElement;
  const v = el.value as TaskState;
  if (v !== "todo" && openBlockers(t) > 0) {
    el.value = t.st;
    shakeId.value = t.id;
    setTimeout(() => (shakeId.value = ""), 400);
    return;
  }
  t.st = v;
  persist();
}

// ---- drawer events -------------------------------------------------------
function onAddLink(name: string, url: string) {
  if (!drawerCtx.value) return;
  drawerCtx.value.item.links.push({ id: uid(), name, url, at: Date.now() });
  persist();
}
function onRemoveLink(id: string) {
  if (!drawerCtx.value) return;
  drawerCtx.value.item.links = drawerCtx.value.item.links.filter((l) => l.id !== id);
  persist();
}
function onAddComment(text: string) {
  if (!drawerCtx.value) return;
  const by = auth.user && auth.user.username ? auth.user.username : "Me";
  drawerCtx.value.item.cm.push({ id: uid(), by, at: Date.now(), text });
  persist();
}
function onRemoveComment(id: string) {
  if (!drawerCtx.value) return;
  drawerCtx.value.item.cm = drawerCtx.value.item.cm.filter((c) => c.id !== id);
  persist();
}

// ---- drag and drop -------------------------------------------------------
function startParentDrag(e: DragEvent, id: string) {
  dragParent.value = id;
  dragTask.value = "";
  e.dataTransfer?.setData("text/plain", "parent");
}
function startTaskDrag(e: DragEvent, id: string) {
  dragTask.value = id;
  dragParent.value = "";
  e.dataTransfer?.setData("text/plain", "task");
}
function clearDrag() {
  dragTask.value = "";
  dragParent.value = "";
  overQuad.value = "";
  overCard.value = "";
}
function onQuadOver(e: DragEvent, q: string) {
  if (!dragParent.value) return;
  e.preventDefault();
  overQuad.value = q;
}
function onCardOver(e: DragEvent, id: string) {
  if (!dragTask.value) return;
  e.preventDefault();
  overCard.value = id;
}
function dropOnQuad(e: DragEvent, q: Quad) {
  if (!dragParent.value || !project.value) return;
  e.preventDefault();
  const p = project.value.parents.find((x) => x.id === dragParent.value);
  clearDrag();
  if (p && p.quad !== q) {
    p.quad = q;
    persist();
  }
}
function dropOnCard(e: DragEvent, targetId: string) {
  if (!dragTask.value || !project.value) return;
  e.preventDefault();
  e.stopPropagation();
  const taskId = dragTask.value;
  clearDrag();
  const from = project.value.parents.find((p) => p.tasks.some((t) => t.id === taskId));
  const to = project.value.parents.find((p) => p.id === targetId);
  if (!from || !to || from === to) return;
  const t = from.tasks.find((x) => x.id === taskId);
  if (!t) return;
  from.tasks = from.tasks.filter((x) => x !== t);
  to.tasks.push(t);
  selId.value = to.id;
  persist();
}

async function deleteProject() {
  if (!project.value) return;
  if (!armDelete.value) {
    armDelete.value = true;
    setTimeout(() => (armDelete.value = false), 4000);
    return;
  }
  try {
    await store.remove(project.value.id);
    router.push("/projects");
  } catch (e: any) {
    $showError(e);
  }
}

watch(
  project,
  (p) => {
    if (p && !p.parents.some((x) => x.id === selId.value)) {
      const want = typeof route.query.sel === "string" ? route.query.sel : "";
      const hit = p.parents.find((x) => x.id === want);
      selId.value = hit ? hit.id : p.parents.length ? p.parents[0].id : "";
    }
  },
  { immediate: true }
);

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
