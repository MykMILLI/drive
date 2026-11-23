<template>
  <section class="card desktop-connect">
    <div class="card-title"><h2>Connect Drive Desktop</h2></div>
    <div class="card-content">
      <p v-if="error" role="alert" class="connect-error">{{ error }}</p>
      <p v-if="loading">Loading connection request...</p>
      <template v-else-if="request && !done">
        <p>A desktop application named <strong>{{ request.name }}</strong> is asking to connect.</p>
        <p>Approve only if you just clicked Connect in your own desktop app. The displayed name is supplied by that app and is not proof of identity.</p>
        <p>Signed in as: <strong>{{ auth.user?.username }}</strong></p>
        <p>This preview can create files and folders in both directions after explicit approval, including downloading existing remote files missing locally. Existing files are never replaced or deleted. Edits to existing files are not synchronized yet.</p>
        <form @submit.prevent="approve">
          <label for="desktop-root">Existing folder in your Drive account</label>
          <input id="desktop-root" v-model.trim="root" class="input" required placeholder="/general/DesktopSync" :disabled="busy || request.approved" />
          <p>This is a Drive folder path, not a Linux host path. The folder must already exist. Folder browsing will replace this input in the completed setup.</p>
          <label><input v-model="allowUploads" type="checkbox" :disabled="busy || request.approved" /> Allow two-way creation of files and folders, including downloading existing remote files (no overwrites or deletions)</label>
          <button class="button" type="submit" :disabled="busy || request.approved">{{ busy ? "Connecting..." : "Approve this computer" }}</button>
          <button class="button" type="button" :disabled="busy" @click="router.push('/files/')">Cancel</button>
        </form>
        <p v-if="request.approved">Already approved. Return to the desktop app.</p>
        <small>Request expires: {{ new Date(request.expiresAt).toLocaleString() }}</small>
      </template>
      <p v-if="done" role="status">Computer approved. Return to Drive Desktop to finish setup. You can close this tab.</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { fetchJSON } from "@/api/utils";
import { useAuthStore } from "@/stores/auth";
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const request = ref<{ name: string; expiresAt: string; approved: boolean } | null>(null);
const root = ref("");
const allowUploads = ref(false);
const error = ref("");
const loading = ref(true);
const busy = ref(false);
const done = ref(false);
const id = typeof route.query.request === "string" ? route.query.request : "";
function describe(e: unknown) { return e instanceof Error ? e.message : "Request failed."; }
onMounted(async () => {
  try {
    if (!/^[a-f0-9]{32}$/.test(id)) throw new Error("Invalid connection request. Start again from the desktop app.");
    request.value = await fetchJSON(`/api/sync/pair/request?id=${encodeURIComponent(id)}`);
  } catch (e) { error.value = describe(e); }
  finally { loading.value = false; }
});
async function approve() {
  if (busy.value || !request.value || request.value.approved) return;
  busy.value = true;
  error.value = "";
  try {
    await fetchJSON("/api/sync/pair/approve", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id, root: root.value, allowUploads: allowUploads.value }),
    });
    done.value = true;
  } catch (e) { error.value = describe(e); }
  finally { busy.value = false; }
}
</script>

<style scoped>
.desktop-connect { max-width: 800px; margin: 1rem auto; }
label { display: block; margin: 1rem 0 0.5rem; }
input:not([type="checkbox"]) { width: 100%; box-sizing: border-box; }
input[type="checkbox"] { width: auto; margin-right: 0.5rem; }
.connect-error { color: #c62828; white-space: pre-wrap; }
</style>
