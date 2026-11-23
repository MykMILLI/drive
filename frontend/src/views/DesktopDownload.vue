<template>
 <section class="card desktop-download">
  <div class="card-title"><h2>Drive Desktop for Windows</h2></div>
  <div class="card-content">
   <p>Download the agent preconfigured for this Drive instance. Open it and click Connect this computer, then approve the connection in your browser. No server address or device token entry is needed.</p>
   <p><strong>Two-way new-item preview:</strong> creates files and folders recursively in both directions, up to 32 MiB per file. Existing-file edits and deletions are NOT synchronized. This unsigned portable package is not a production sync client.</p>
   <p v-if="error" role="alert" class="download-error">{{ error }}</p>
   <p v-if="loading">Checking agent availability...</p>
   <template v-else>
    <p v-if="!available">The administrator has not published the desktop executable and instance configuration yet.</p>
    <p v-if="server">Instance: {{ server }}</p>
    <p v-if="server.startsWith('http://')" class="download-error">This instance uses unencrypted HTTP. Use only for a trusted-LAN development test. Use HTTPS before wider deployment.</p>
    <button class="button" :disabled="!available || busy" @click="download">{{ busy ? "Preparing download..." : "Download Windows agent" }}</button>
   </template>
   <ol><li>Extract the entire downloaded folder to a permanent location.</li><li>Run DriveDesktop.exe.</li><li>Click Connect this computer and approve it in your browser.</li><li>Select a disposable local test folder. Reconnect and explicitly approve two-way file/folder transfers before testing.</li></ol>
  </div>
 </section>
</template>
<script setup lang="ts">
import { onMounted, ref } from "vue";
import { fetchJSON, fetchURL } from "@/api/utils";
const available = ref(false);
const server = ref("");
const loading = ref(true);
const busy = ref(false);
const error = ref("");
function describe(e: unknown) { return e instanceof Error ? e.message : "Request failed."; }
onMounted(async () => {
 try {
  const result = await fetchJSON<{ available: boolean; server: string }>("/api/desktop/status");
  available.value = result.available;
  server.value = result.server;
 } catch (e) { error.value = describe(e); }
 finally { loading.value = false; }
});
async function download() {
 if (busy.value) return;
 busy.value = true;
 error.value = "";
 try {
  const response = await fetchURL("/api/desktop/download", {});
  const url = URL.createObjectURL(await response.blob());
  const link = document.createElement("a");
  link.href = url;
  link.download = "DriveDesktop-Windows.zip";
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 30000);
 } catch (e) { error.value = describe(e); }
 finally { busy.value = false; }
}
</script>
<style scoped>
.desktop-download { max-width: 850px; margin: 1rem auto; }
.download-error { color: #c62828; white-space: pre-wrap; }
li { margin: 0.6rem 0; }
</style>
