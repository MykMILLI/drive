import { fetchJSON, fetchURL } from "./utils";
import { baseURL } from "@/utils/constants";

export interface SyncDevice {
  id: string;
  name: string;
  root: string;
  createdAt: string;
  expiresAt: string;
}

export interface CreatedSyncDevice {
  device: SyncDevice;
  token: string;
}

export interface SyncInventory {
  protocol: number;
  root: string;
  entries: Array<{
    path: string;
    isDir: boolean;
    size: number;
    modified: string;
  }>;
  complete: boolean;
  deletionSafe: boolean;
}

export function listDevices() {
  return fetchJSON<SyncDevice[]>("/api/sync/devices");
}

export function registerDevice(name: string, root: string) {
  return fetchJSON<CreatedSyncDevice>("/api/sync/devices", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, root }),
  });
}

export async function revokeDevice(id: string) {
  await fetchURL(`/api/sync/devices?id=${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

// Deliberately sends only the device credential, not the account session.
// This verifies that the device can authenticate independently.
export async function testDevice(token: string): Promise<SyncInventory> {
  const response = await fetch(`${baseURL}/api/sync/device/inventory`, {
    method: "GET",
    headers: { Authorization: `Bearer ${token}` },
    credentials: "omit",
    cache: "no-store",
    redirect: "error",
  });
  if (!response.ok) {
    throw new Error(
      `Device test failed (${response.status}). Check the folder, permissions, and credential.`
    );
  }
  const result = (await response.json()) as SyncInventory;
  if (result.protocol !== 1 || !Array.isArray(result.entries) || result.complete !== true) {
    throw new Error("The server returned an unsupported or incomplete inventory.");
  }
  return result;
}
