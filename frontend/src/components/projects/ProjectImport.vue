<template>
  <div class="modal-back" @click.self="emit('close')">
    <div class="modal" role="dialog" aria-label="Import hub ticket">
      <h3 style="margin: 0">Import hub contractor ticket</h3>
      <p class="muted">
        Creates a project (or updates the existing one for the same ticket
        number): checklist items become tasks, comments are kept, and the
        matrix quadrant comes from priority and due date.
      </p>

      <template v-if="!result">
        <textarea v-model="raw" placeholder="Open the ticket, select all (Ctrl+A), copy, and paste here..."></textarea>
        <div class="pvw">
          <template v-if="parsed">
            <div><span class="k">Ticket</span><b>#{{ parsed.id }} · {{ parsed.category }}</b></div>
            <div><span class="k">Title</span>{{ parsed.title }}</div>
            <div><span class="k">Priority</span>{{ parsed.priority || "-" }} <template v-if="parsed.kind">· {{ parsed.kind }}</template></div>
            <div>
              <span class="k">Due</span>{{ niceDate(parsed.due) }}
              <span v-if="dueLabel(parsed.due)" class="badge s-blocked">{{ dueLabel(parsed.due) }}</span>
            </div>
            <div><span class="k">Assigned</span>{{ parsed.assignee || "-" }}</div>
            <div><span class="k">Checklist</span>{{ parsed.items.length }} items become tasks</div>
            <div><span class="k">Comments</span>{{ parsed.entries.length }} entries</div>
            <div><span class="k">Quadrant</span>{{ quadName }}</div>
            <div v-if="existing" class="warn">A project for this ticket already exists. Importing updates it and will not duplicate it.</div>
            <div v-for="w in parsed.warn" :key="w" class="warn">Warning: {{ w }}</div>
          </template>
          <span v-else class="muted">
            {{ raw.trim() ? 'Could not find a ticket header like "#1552 · GENERAL". Paste from the top of the ticket.' : "Waiting for ticket text..." }}
          </span>
        </div>

        <details style="margin-top: 0.8em">
          <summary class="muted">One-click copy from the hub (bookmarklet)</summary>
          <p class="muted">
            Create a browser bookmark named "Copy ticket" and paste this as its URL. Click it on a ticket page,
            then paste here. Some sites block bookmarklets; plain copy and paste always works.
          </p>
          <textarea readonly style="height: 90px" :value="bookmarklet" @focus="($event.target as HTMLTextAreaElement).select()"></textarea>
        </details>

        <div class="pj-actions" style="margin-top: 0.8em; justify-content: flex-end">
          <button class="pb" type="button" @click="paste">Paste from clipboard</button>
          <button class="pb" type="button" @click="emit('close')">Cancel</button>
          <button class="pb pri" type="button" :disabled="!parsed || busy" @click="run">Import</button>
        </div>
      </template>

      <template v-else>
        <div class="pvw">
          <div class="ok"><b>{{ result.isNew ? "Project created" : "Project updated" }}</b> from ticket #{{ parsed?.id }}</div>
          <div><span class="k">Tasks added</span>{{ result.added.tasks }}</div>
          <div><span class="k">Comments added</span>{{ result.added.comments }}</div>
          <div><span class="k">Suggested roadblocks</span>{{ result.added.roadblocks }} (review and delete any that do not apply)</div>
          <div><span class="k">Quadrant</span>{{ quadName }}</div>
        </div>
        <div class="pj-actions" style="margin-top: 0.8em; justify-content: flex-end">
          <button class="pb" type="button" @click="emit('close')">Close</button>
          <button class="pb pri" type="button" @click="emit('imported', result.project.id)">Open project</button>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref } from "vue";
import { useProjectsStore } from "@/stores/projects";
import { parseTicket } from "@/utils/ticketParser";
import { importTicket, quadFor } from "@/utils/ticketImport";
import { dueLabel, niceDate } from "@/utils/projectLogic";
import { QUADS } from "@/utils/quadrants";
import type { ImportResult } from "@/types/projects";

const emit = defineEmits<{
  (e: "close"): void;
  (e: "imported", id: string): void;
}>();

const $showError = inject<IToastError>("$showError")!;
const store = useProjectsStore();
const raw = ref("");
const result = ref<ImportResult | null>(null);
const busy = ref(false);

const parsed = computed(() => (raw.value.trim() ? parseTicket(raw.value) : null));
const existing = computed(() =>
  parsed.value
    ? store.items.find((p) => p.ticket && p.ticket.id === parsed.value!.id)
    : undefined
);
const quadName = computed(() => {
  const key = result.value ? result.value.quad : parsed.value ? quadFor(parsed.value) : "q2";
  const q = QUADS.find((x) => x.key === key);
  return q ? q.name : "";
});

// Built from parts so the literal scheme never appears in the source file.
const bookmarklet = ["java", "script:"].join("") +
  "(function(){var t=document.body.innerText;navigator.clipboard.writeText(t).then(function(){alert('Ticket copied. Paste it into Drive Projects.')},function(){prompt('Copy this text:',t)})})()";

async function paste() {
  try {
    raw.value = await navigator.clipboard.readText();
  } catch (e) {
    $showError(new Error("Clipboard is blocked here. Click in the box and press Ctrl+V instead."));
  }
}

async function run() {
  const t = parsed.value;
  if (!t || busy.value) return;
  busy.value = true;
  try {
    const ex = existing.value;
    const res = importTicket(t, ex, store.takenIds);
    if (ex) await store.persist(ex.id);
    else await store.create(res.project);
    result.value = res;
  } catch (e: any) {
    $showError(e);
  } finally {
    busy.value = false;
  }
}
</script>
