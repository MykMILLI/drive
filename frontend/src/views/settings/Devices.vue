<template>
  <section class="desktop-sync-settings">
    <div class="card">
      <div class="card-title">
        <h2>Desktop sync</h2>
      </div>
      <div class="card-content">
        <p>
          Register a desktop for one existing folder. Device credentials can
          currently read folder inventories only. File transfers and automatic
          synchronization are still under development.
        </p>
        <p v-if="error" class="sync-error" role="alert">{{ error }}</p>
        <p v-if="notice" role="status">{{ notice }}</p>

        <form @submit.prevent="create">
          <label for="sync-device-name">Device name</label>
          <input
            id="sync-device-name"
            v-model="deviceName"
            class="input"
            maxlength="100"
            required
            placeholder="My Windows desktop"
            :disabled="busy || !!credential"
          />

          <label for="sync-device-root">Server folder</label>
          <input
            id="sync-device-root"
            v-model="remoteRoot"
            class="input"
            maxlength="4096"
            required
            placeholder="/general/DesktopSync"
            :disabled="busy || !!credential"
          />
          <p class="sync-hint">
            Use a folder path as it appears inside your Drive account, not a
            Linux host path. The folder must already exist. The root / cannot
            be selected.
          </p>

          <button
            type="submit"
            class="button"
            :disabled="busy || !!credential || !canDownload"
          >
            {{ busy ? "Working…" : "Register device" }}
          </button>
          <p v-if="!canDownload" class="sync-hint">
            Your account needs download permission to register a device.
          </p>
        </form>
      </div>
    </div>

    <div v-if="credential" class="card sync-credential">
      <div class="card-title"><h3>New device credential</h3></div>
      <div class="card-content">
        <p>
          This secret is shown only during this visit. Keep it private: anyone
          holding it can use the approved device access. Leaving this page
          clears the displayed secret but does not revoke the device.
        </p>
        <label for="sync-credential">Credential</label>
        <textarea
          id="sync-credential"
          :value="credential"
          readonly
          spellcheck="false"
          rows="3"
          autocomplete="off"
        ></textarea>

        <div class="sync-actions">
          <button type="button" class="button" @click="copyCredential">
            Copy credential
          </button>
          <button
            type="button"
            class="button"
            :disabled="busy"
            @click="verify"
          >
            Test device connection
          </button>
          <button
            type="button"
            class="button"
            :disabled="busy"
            @click="dismissCredential"
          >
            Dismiss secret
          </button>
        </div>

        <p v-if="testResult" role="status">{{ testResult }}</p>
      </div>
    </div>

    <div class="card">
      <div class="card-title"><h3>Registered devices</h3></div>
      <div class="card-content">
        <button
          type="button"
          class="button"
          :disabled="busy"
          @click="refresh"
        >
          Refresh
        </button>
        <p v-if="!devices.length && !busy">No registered devices.</p>

        <ul class="sync-device-list">
          <li v-for="device in devices" :key="device.id">
            <div>
              <strong>{{ device.name }}</strong>
              <code>{{ device.root }}</code>
              <small>
                {{ expired(device) ? "Expired" : "Expires" }}:
                {{ formatDate(device.expiresAt) }}
              </small>
            </div>
            <button
              type="button"
              class="button"
              :disabled="busy"
              @click="revoke(device)"
            >
              Revoke
            </button>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { onBeforeRouteLeave } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import {
  listDevices,
  registerDevice,
  revokeDevice,
  testDevice,
  type SyncDevice,
} from "@/api/desktopSync";

const auth = useAuthStore();
const canDownload = computed(() => !!auth.user?.perm.download);
const devices = ref<SyncDevice[]>([]);
const deviceName = ref("");
const remoteRoot = ref("");
const credential = ref("");
const credentialDeviceId = ref("");
const busy = ref(false);
const error = ref("");
const notice = ref("");
const testResult = ref("");

function message(value: unknown): string {
  return value instanceof Error ? value.message : "The request failed.";
}

function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function expired(device: SyncDevice): boolean {
  return new Date(device.expiresAt).getTime() <= Date.now();
}

async function refresh() {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    devices.value = await listDevices();
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

async function create() {
  if (busy.value || credential.value) return;
  busy.value = true;
  error.value = "";
  notice.value = "";
  testResult.value = "";
  try {
    const result = await registerDevice(
      deviceName.value.trim(),
      remoteRoot.value.trim()
    );
    credential.value = result.token;
    credentialDeviceId.value = result.device.id;
    devices.value = [
      result.device,
      ...devices.value.filter((device) => device.id !== result.device.id),
    ];
    notice.value = "Device registered. Test its connection below.";
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

async function copyCredential() {
  error.value = "";
  try {
    await navigator.clipboard.writeText(credential.value);
    notice.value = "Credential copied. Do not paste it into chat or logs.";
  } catch {
    error.value =
      "Clipboard access is unavailable. Select and copy the credential manually.";
  }
}

async function verify() {
  if (busy.value || !credential.value) return;
  busy.value = true;
  error.value = "";
  testResult.value = "";
  try {
    const result = await testDevice(credential.value);
    const files = result.entries.filter((entry) => !entry.isDir).length;
    const folders = result.entries.filter((entry) => entry.isDir).length;
    testResult.value =
      `Connected to ${result.root}: ${files} files and ${folders} folders. ` +
      "Inventory only; no files were transferred.";
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

function dismissCredential() {
  if (
    credential.value &&
    !window.confirm(
      "Clear this displayed credential? It cannot be retrieved later. " +
      "The registered device will remain active until revoked or expired."
    )
  ) return;
  credential.value = "";
  credentialDeviceId.value = "";
  testResult.value = "";
  notice.value = "Displayed secret cleared.";
}

async function revoke(device: SyncDevice) {
  if (
    busy.value ||
    !window.confirm(`Revoke access for "${device.name}"?`)
  ) return;
  busy.value = true;
  error.value = "";
  try {
    await revokeDevice(device.id);
    devices.value = devices.value.filter((item) => item.id !== device.id);
    if (credentialDeviceId.value === device.id) {
      credential.value = "";
      credentialDeviceId.value = "";
      testResult.value = "";
    }
    notice.value = "Device revoked.";
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

onBeforeRouteLeave(() => {
  if (busy.value) return false;
  if (!credential.value) return true;
  return window.confirm(
    "Leave this page and clear the displayed device credential? " +
    "The device itself will remain registered."
  );
});

onMounted(refresh);
</script>

<style scoped>
.desktop-sync-settings {
  max-width: 960px;
  margin: 0 auto;
}
label {
  display: block;
  margin: 1rem 0 0.4rem;
  font-weight: 600;
}
.input,
textarea {
  box-sizing: border-box;
  width: 100%;
}
textarea {
  padding: 0.75rem;
  font-family: monospace;
  overflow-wrap: anywhere;
}
.sync-hint,
small {
  opacity: 0.8;
}
.sync-error {
  color: #c62828;
  white-space: pre-wrap;
}
.sync-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.6rem;
  margin-top: 1rem;
}
.sync-device-list {
  list-style: none;
  padding: 0;
}
.sync-device-list li {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  padding: 1rem 0;
  border-bottom: 1px solid #8884;
}
.sync-device-list code,
.sync-device-list small {
  display: block;
  margin-top: 0.4rem;
  overflow-wrap: anywhere;
}
</style>
