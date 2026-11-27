<template>
  <div class="wk">
    <aside class="wk-side" aria-label="Wiki pages">
      <div class="wk-sh">
        <input v-model="filter" class="wk-filter" placeholder="Filter pages" aria-label="Filter pages" />
        <button class="pb pri" type="button" @click="begin('Untitled page', '', '')">+ Page</button>
      </div>
      <div v-if="loading" class="muted">Loading...</div>
      <ul v-else class="wk-tree">
        <li v-for="n in shown" :key="n.page.id">
          <button
            type="button"
            class="wk-item"
            :class="{ sel: n.page.id === selId }"
            :style="{ paddingLeft: 0.6 + n.depth * 1.1 + 'em' }"
            @click="select(n.page.id)"
          >
            {{ n.page.title }}
          </button>
        </li>
      </ul>
      <p v-if="!loading && !pages.length" class="muted">No pages yet.</p>
      <p v-else-if="!loading && filter && !shown.length" class="muted">No page matches.</p>
    </aside>

    <section class="wk-main">
      <div v-if="loading" class="empty-state">
        <div class="spinner"><div class="bounce1"></div><div class="bounce2"></div><div class="bounce3"></div></div>
      </div>

      <div v-else-if="!draft" class="empty-state">
        <template v-if="!pages.length">
          <h3>This project has no wiki yet</h3>
          <p>
            Start with a home page for notes, decisions and runbooks. Pages are saved in this project's Drive
            folder, so they sync and back up along with everything else.
          </p>
          <button class="pb pri" type="button" @click="startHome">Create home page</button>
        </template>
        <p v-else class="muted">Select a page.</p>
      </div>

      <template v-else>
        <nav v-if="trail.length" class="wk-crumbs" aria-label="Breadcrumb">
          <template v-for="p in trail" :key="p.id">
            <button type="button" class="wk-crumb" @click="select(p.id)">{{ p.title }}</button>
            <span aria-hidden="true">›</span>
          </template>
        </nav>

        <div class="wk-head">
          <input
            v-model="draft.title"
            class="wk-title"
            :readonly="!editing"
            maxlength="200"
            placeholder="Page title"
            aria-label="Page title"
          />
          <div class="pj-actions">
            <template v-if="editing">
              <button class="pb pri" type="button" :disabled="saving || !(isNew || dirty)" @click="save">
                {{ saving ? "Saving..." : "Save" }}
              </button>
              <button class="pb" type="button" @click="cancelEdit">Cancel</button>
            </template>
            <template v-else>
              <button class="pb pri" type="button" @click="editing = true">Edit</button>
              <button class="pb" type="button" @click="begin('Untitled page', draft.id, '')">+ Child page</button>
              <button class="pb" type="button" @click="toggleHistory">History</button>
              <button class="pb" :class="{ danger: armDelete }" type="button" @click="remove">
                {{ armDelete ? "Click again to confirm" : "Delete" }}
              </button>
            </template>
          </div>
        </div>

        <div class="muted wk-status">
          <span v-if="isNew">New page - not saved yet</span>
          <span v-else-if="dirty">Unsaved changes</span>
          <span v-else-if="savedNote">{{ savedNote }}</span>
          <span v-else-if="draft.updatedAt">Last saved {{ niceStamp(draft.updatedAt) }} · version {{ draft.version }}</span>
        </div>

        <div v-if="conflict" class="wk-conflict" role="alert">
          Someone saved this page first, so your copy is out of date.
          <button class="pb" type="button" @click="reload">Reload latest</button>
        </div>

        <div v-if="editing" class="meta wk-meta">
          Parent page
          <select v-model="draft.parentId" aria-label="Parent page">
            <option value="">(top level)</option>
            <option v-for="p in parentOptions" :key="p.id" :value="p.id">{{ p.title }}</option>
          </select>
          <template v-if="project.parents.length">
            Documents
            <label v-for="c in project.parents" :key="c.id" class="wk-ref">
              <input v-model="draft.refs" type="checkbox" :value="c.id" /> {{ c.name }}
            </label>
          </template>
        </div>
        <div v-else-if="linkedCards.length" class="meta wk-meta">
          Documents
          <span v-for="c in linkedCards" :key="c.id" class="badge s-todo">{{ c.name }}</span>
        </div>

        <div v-if="showHistory" class="wk-history">
          <h4>Earlier versions</h4>
          <p v-if="!revisions.length" class="muted">No earlier versions yet. One is kept each time the page is saved.</p>
          <div v-for="r in revisions" :key="r.version" class="wk-rev">
            <span>Version {{ r.version }} · {{ niceStamp(r.updatedAt) }}</span>
            <button class="pb" type="button" @click="viewRevision(r.version)">View</button>
          </div>
          <div v-if="viewingRev" class="wk-revview">
            <div class="wk-revhead">
              <b>Version {{ viewingRev.version }}: {{ viewingRev.title }}</b>
              <button class="pb pri" type="button" @click="restore">Restore into editor</button>
              <button class="x" type="button" aria-label="Close version" @click="viewingRev = null">✕</button>
            </div>
            <!-- eslint-disable-next-line vue/no-v-html -->
            <div class="wk-md" v-html="revHtml"></div>
          </div>
        </div>

        <div v-if="editing" class="wk-edit">
          <textarea
            v-model="draft.body"
            class="wk-ta"
            spellcheck="true"
            placeholder="Write in Markdown. Link other pages with [[Page title]]."
            aria-label="Page content (Markdown)"
            @keydown="onKey"
          ></textarea>
          <!-- eslint-disable-next-line vue/no-v-html -->
          <div class="wk-prev wk-md" @click="onContentClick" v-html="html"></div>
        </div>
        <!-- eslint-disable-next-line vue/no-v-html -->
        <div v-else class="wk-md" @click="onContentClick" v-html="html"></div>
      </template>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import * as api from "@/api/wiki";
import { useWikiStore } from "@/stores/wiki";
import { ancestors, buildTree, descendantIds, newPageId, starterBody, wikiIdFromHref } from "@/utils/wikiLogic";
import { renderWiki } from "@/utils/wikiRender";
import type { Project } from "@/types/projects";
import type { WikiPage, WikiRevision } from "@/types/wiki";

const props = defineProps<{ project: Project }>();

const $showError = inject<IToastError>("$showError")!;
const route = useRoute();
const router = useRouter();
const wiki = useWikiStore();

const loading = ref(true);
const filter = ref("");
const selId = ref("");
const draft = ref<WikiPage | null>(null);
const editing = ref(false);
const saving = ref(false);
const conflict = ref(false);
const armDelete = ref(false);
const showHistory = ref(false);
const revisions = ref<WikiRevision[]>([]);
const viewingRev = ref<WikiPage | null>(null);
const savedNote = ref("");

const pages = computed(() => wiki.forProject(props.project.id));
const tree = computed(() => buildTree(pages.value));
const shown = computed(() => {
  const q = filter.value.trim().toLowerCase();
  if (!q) return tree.value;
  return tree.value
    .filter((n) => n.page.title.toLowerCase().includes(q))
    .map((n) => ({ ...n, depth: 0 }));
});

const stored = computed(() => pages.value.find((p) => p.id === selId.value));
const isNew = computed(() => !!draft.value && draft.value.version === 0);
// Title and body a new page started with, so Cancel only nags when the person
// has actually typed something. A new page can always be saved as it stands.
const seed = ref("");
const dirty = computed(() => {
  const d = draft.value;
  if (!d) return false;
  if (isNew.value) return d.title + "\u0000" + (d.body ?? "") !== seed.value;
  const s = stored.value;
  if (!s) return false;
  return (
    d.title !== s.title ||
    (d.body ?? "") !== (s.body ?? "") ||
    d.parentId !== s.parentId ||
    d.refs.join(",") !== s.refs.join(",")
  );
});

const trail = computed(() => (draft.value && !isNew.value ? ancestors(pages.value, draft.value.id) : []));
const parentOptions = computed(() => {
  const d = draft.value;
  if (!d) return [];
  const blocked = isNew.value ? new Set<string>() : descendantIds(pages.value, d.id);
  return pages.value.filter((p) => p.id !== d.id && !blocked.has(p.id));
});
const linkedCards = computed(() =>
  draft.value ? props.project.parents.filter((c) => draft.value!.refs.includes(c.id)) : []
);

const html = computed(() => renderWiki(draft.value?.body ?? "", pages.value));
const revHtml = computed(() => renderWiki(viewingRev.value?.body ?? "", pages.value));

const clone = (p: WikiPage): WikiPage => JSON.parse(JSON.stringify(p));

function niceStamp(iso: string): string {
  const d = new Date(iso);
  return isNaN(d.getTime()) ? iso : d.toLocaleString();
}

let noteTimer: ReturnType<typeof setTimeout> | undefined;
function flash(msg: string) {
  savedNote.value = msg;
  clearTimeout(noteTimer);
  noteTimer = setTimeout(() => (savedNote.value = ""), 3000);
}

function syncRoute(id: string) {
  const query = { ...route.query, tab: "wiki" } as Record<string, any>;
  if (id) query.page = id;
  else delete query.page;
  router.replace({ query });
}

function confirmDiscard(): boolean {
  return !dirty.value || window.confirm("Discard your unsaved changes to this page?");
}

function resetPanels() {
  conflict.value = false;
  armDelete.value = false;
  showHistory.value = false;
  viewingRev.value = null;
}

async function select(id: string, force = false) {
  if (!force && !confirmDiscard()) return;
  try {
    const full = await wiki.open(props.project.id, id);
    selId.value = id;
    draft.value = clone(full);
    editing.value = false;
    resetPanels();
    syncRoute(id);
  } catch (e: any) {
    $showError(e);
  }
}

/** Starts a brand-new, unsaved page. Its id is derived from the title when it is first saved. */
function begin(title: string, parentId: string, body: string) {
  if (!confirmDiscard()) return;
  selId.value = "";
  draft.value = { id: "", title, parentId, version: 0, kind: "page", refs: [], body };
  seed.value = title + "\u0000" + body;
  editing.value = true;
  resetPanels();
  syncRoute("");
}

function startHome() {
  begin(props.project.name, "", starterBody(props.project));
}

async function save() {
  const d = draft.value;
  if (!d) return;
  if (!d.title.trim()) {
    $showError(new Error("Give the page a title."));
    return;
  }
  saving.value = true;
  try {
    const out = clone(d);
    out.title = out.title.trim();
    out.kind = out.kind || "page";
    if (out.version === 0) {
      out.id = newPageId(
        out.title,
        pages.value.map((p) => p.id)
      );
    }
    const saved = await wiki.save(props.project.id, out);
    selId.value = saved.id;
    draft.value = clone(saved);
    editing.value = false;
    conflict.value = false;
    flash("Saved");
    syncRoute(saved.id);
  } catch (e: any) {
    if (e && e.status === 409) conflict.value = true;
    else $showError(e);
  } finally {
    saving.value = false;
  }
}

function cancelEdit() {
  if (!confirmDiscard()) return;
  if (isNew.value || !stored.value) {
    draft.value = null;
    selId.value = "";
    editing.value = false;
    const first = tree.value[0]?.page.id;
    if (first) select(first, true);
    else syncRoute("");
    return;
  }
  draft.value = clone(stored.value);
  editing.value = false;
  conflict.value = false;
}

async function reload() {
  try {
    await wiki.load(props.project.id, true);
    if (selId.value && pages.value.some((p) => p.id === selId.value)) {
      await select(selId.value, true);
    } else {
      draft.value = null;
      selId.value = "";
      editing.value = false;
      conflict.value = false;
    }
  } catch (e: any) {
    $showError(e);
  }
}

async function remove() {
  const d = draft.value;
  if (!d || isNew.value) return;
  if (!armDelete.value) {
    armDelete.value = true;
    setTimeout(() => (armDelete.value = false), 4000);
    return;
  }
  try {
    await wiki.remove(props.project.id, d.id);
    draft.value = null;
    selId.value = "";
    armDelete.value = false;
    const first = tree.value[0]?.page.id;
    if (first) await select(first, true);
    else syncRoute("");
  } catch (e: any) {
    armDelete.value = false;
    $showError(e);
  }
}

async function toggleHistory() {
  showHistory.value = !showHistory.value;
  viewingRev.value = null;
  if (!showHistory.value || !draft.value) return;
  try {
    revisions.value = await api.history(props.project.id, draft.value.id);
  } catch (e: any) {
    $showError(e);
  }
}

async function viewRevision(version: number) {
  if (!draft.value) return;
  try {
    viewingRev.value = await api.revision(props.project.id, draft.value.id, version);
  } catch (e: any) {
    $showError(e);
  }
}

/** Puts an old version into the editor. Nothing is saved until the person presses Save. */
function restore() {
  if (!draft.value || !viewingRev.value) return;
  draft.value.title = viewingRev.value.title;
  draft.value.body = viewingRev.value.body ?? "";
  editing.value = true;
  showHistory.value = false;
  viewingRev.value = null;
}

function onKey(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
    e.preventDefault();
    save();
  }
}

function onContentClick(e: MouseEvent) {
  const a = (e.target as HTMLElement).closest("a");
  const id = wikiIdFromHref(a?.getAttribute("href"));
  if (!id) return;
  e.preventDefault();
  if (pages.value.some((p) => p.id === id)) select(id);
}

const warnUnsaved = (e: BeforeUnloadEvent) => {
  if (dirty.value) {
    e.preventDefault();
    e.returnValue = "";
  }
};

watch(
  () => route.query.page,
  (id) => {
    if (typeof id === "string" && id && id !== selId.value && pages.value.some((p) => p.id === id)) {
      select(id);
    }
  }
);

onMounted(async () => {
  window.addEventListener("beforeunload", warnUnsaved);
  try {
    await wiki.load(props.project.id, true);
  } catch (e: any) {
    $showError(e);
    loading.value = false;
    return;
  }
  loading.value = false;
  const want = typeof route.query.page === "string" ? route.query.page : "";
  const first = pages.value.some((p) => p.id === want) ? want : tree.value[0]?.page.id ?? "";
  if (first) await select(first, true);
});

onBeforeUnmount(() => {
  window.removeEventListener("beforeunload", warnUnsaved);
  clearTimeout(noteTimer);
});
</script>
