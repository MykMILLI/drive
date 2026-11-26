<template>
  <aside class="drawer" role="dialog" :aria-label="title">
    <div class="dh">
      <div>
        <b>{{ title }}</b>
        <div class="muted">{{ sub }}</div>
      </div>
      <button class="x" type="button" aria-label="Close" @click="emit('close')">✕</button>
    </div>

    <div class="db">
      <h4>Links and files · {{ links.length }}</h4>
      <p v-if="folder" class="muted">
        Upload files to
        <router-link :to="folder">this project's Drive folder</router-link>
        to get previews and share links. Add web links and videos here.
      </p>
      <div class="lk">
        <input v-model="url" placeholder="Paste a link or video URL" @keyup.enter="addLink" />
        <input v-model="label" placeholder="Label (optional)" @keyup.enter="addLink" />
        <button class="pb" type="button" @click="addLink">Add</button>
      </div>
      <p v-if="linkError" class="warn">{{ linkError }}</p>
      <div v-for="l in links" :key="l.id" class="lnk">
        <a :href="l.url" target="_blank" rel="noopener noreferrer">{{ l.name }}</a>
        <button class="x" type="button" aria-label="Remove link" @click="emit('remove-link', l.id)">✕</button>
      </div>

      <h4>Comments · {{ cm.length }}</h4>
      <div v-for="c in cm" :key="c.id" class="cm" :class="{ act: c.activity }">
        <div class="h">
          <span>{{ c.activity ? "Hub activity" : c.by }} · {{ when(c.at) }}</span>
          <button class="x" type="button" aria-label="Delete comment" @click="emit('remove-comment', c.id)">✕</button>
        </div>
        <p>{{ c.text }}</p>
      </div>
      <div class="cf">
        <textarea
          v-model="text"
          placeholder="Write a comment... (Ctrl+Enter to post)"
          @keydown.ctrl.enter="addComment"
          @keydown.meta.enter="addComment"
        ></textarea>
        <button class="pb pri" type="button" @click="addComment">Post comment</button>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { ref } from "vue";
import type { ProjectComment, ProjectLink } from "@/types/projects";
import { safeUrl } from "@/utils/projectLogic";

defineProps<{
  title: string;
  sub: string;
  links: ProjectLink[];
  cm: ProjectComment[];
  folder?: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "add-link", name: string, url: string): void;
  (e: "remove-link", id: string): void;
  (e: "add-comment", text: string): void;
  (e: "remove-comment", id: string): void;
}>();

const url = ref("");
const label = ref("");
const text = ref("");
const linkError = ref("");

const when = (at: number) =>
  new Date(at).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });

function addLink() {
  const clean = safeUrl(url.value);
  if (!clean) {
    linkError.value = "Enter a valid http(s) link.";
    return;
  }
  linkError.value = "";
  const name = label.value.trim() || new URL(clean).hostname;
  emit("add-link", name, clean);
  url.value = "";
  label.value = "";
}

function addComment() {
  const t = text.value.trim();
  if (!t) return;
  emit("add-comment", t);
  text.value = "";
}
</script>
