<template>
<section class="backup-admin">
 <div class="card"><div class="card-title"><h2>Backup administration</h2></div><div class="card-content">
  <p>Configure a provider-neutral S3 repository and inspect repository jobs. Manual data backups include a consistent Drive database export and selected live folders. Private staging recovery is available for eligible new backups. Scheduling, retention and live application replacement are not enabled. Saving settings does not run a backup.</p>
  <p v-if="!engineAvailable" class="backup-error">Restic is unavailable in this running image. Settings may be saved, but repository operations cannot run.</p>
  <p v-if="error" class="backup-error" role="alert">{{ error }}</p><p v-if="notice" role="status">{{ notice }}</p>
  <button class="button" :disabled="busy" @click="newDestination">New destination</button>
  <button class="button" :disabled="busy" @click="refresh">Refresh</button>
  <div v-for="destination in destinations" :key="destination.config.id" class="destination">
   <h3>{{ destination.config.name }} <small>{{ destination.config.enabled ? 'Enabled' : 'Disabled' }}</small></h3>
   <p>{{ destination.config.endpoint || 'No endpoint configured' }} / {{ destination.config.bucket || 'No bucket configured' }} / {{ destination.config.prefix }}</p>
   <p>Keys: {{ destination.secrets.accessKeySet && destination.secrets.secretKeySet ? 'stored' : 'missing' }}; encryption password: {{ destination.secrets.passwordSet ? 'stored' : 'missing' }}</p>
   <div class="actions">
    <button class="button" :disabled="busy" @click="edit(destination)">Edit</button>
    <button class="button" :disabled="busy || !destination.config.enabled || !engineAvailable || activeJob || !(destination.config.sourcePaths?.length)" @click="run(destination, 'backup')">Back up selected folders + database</button>
    <button class="button" :disabled="busy || !destination.config.enabled || !engineAvailable || activeJob" @click="run(destination, 'snapshots')">List snapshots / test repository</button>
    <button class="button" :disabled="busy || !destination.config.enabled || !engineAvailable || activeJob" @click="run(destination, 'check')">Check repository</button>
    <button class="button" :disabled="busy || !destination.config.enabled || !engineAvailable || activeJob" @click="run(destination, 'initialize')">Initialize repository</button>
   </div>
  </div>
  <p v-if="!destinations.length">No backup destinations configured. You may save a disabled destination before choosing a provider.</p>
 </div></div>
 <div v-if="editing" class="card"><div class="card-title"><h3>{{ form.id ? 'Edit destination' : 'New destination' }}</h3></div><div class="card-content">
  <form @submit.prevent="save">
   <label>Name<input v-model.trim="form.name" class="input" maxlength="100" required /></label>
   <label><input v-model="form.enabled" type="checkbox" /> Enable repository operations</label>
   <label>S3 endpoint<input v-model.trim="form.endpoint" class="input" placeholder="https://your-s3-endpoint" :required="form.enabled" /></label>
   <label>Bucket<input v-model.trim="form.bucket" class="input" :required="form.enabled" /></label>
   <label>Repository prefix<input v-model.trim="form.prefix" class="input" placeholder="millitop/drive" /></label>
   <fieldset class="backup-sources">
    <legend>Drive folders to include in manual data backups</legend>
    <p>Only selected folders are included. The private configuration, live database and broad application-data mounts are excluded; a consistent database export is included separately. Files are read live, so this is not an atomic filesystem-wide snapshot. Use native database/VM backup tools for other live application databases or virtual disks.</p>
    <ul><li v-for="source in (form.sourcePaths || [])" :key="source"><code>{{ source }}</code> <button class="button" type="button" @click="removeSource(source)">Remove</button></li></ul>
    <label>Browse Drive folder<input v-model="browsePath" class="input" /></label>
    <div class="actions"><button class="button" type="button" :disabled="browsing" @click="browse(browsePath)">Browse</button><button class="button" type="button" :disabled="browsing || currentFolder === '/'" @click="parentFolder">Parent</button><button class="button" type="button" :disabled="browsing || !canSelectFolder" @click="addSource(currentFolder)">Select current folder</button></div>
    <p>Current folder: <code>{{ currentFolder }}</code></p>
    <p v-if="browseLimited">Folder listing is limited. Enter a more specific Drive path to browse below it.</p>
    <div v-for="folder in sourceFolders" :key="folder.path" class="actions"><span>{{ folder.name }}</span><button class="button" type="button" :disabled="browsing" @click="browse(folder.path)">Open</button><button class="button" type="button" :disabled="browsing" @click="addSource(folder.path)">Select</button></div>
   </fieldset>
   <label>Region<input v-model.trim="form.region" class="input" placeholder="Provider-specific; optional" /></label>
   <label>Addressing style<select v-model="form.addressing" class="input"><option value="auto">Auto</option><option value="path">Path-style</option><option value="dns">Virtual-hosted</option></select></label>
   <div class="grid"><label>Connections<input v-model.number="form.connections" class="input" type="number" min="1" max="64" /></label><label>Upload KiB/s (0 = unlimited)<input v-model.number="form.uploadKiB" class="input" type="number" min="0" /></label><label>Download KiB/s (0 = unlimited)<input v-model.number="form.downloadKiB" class="input" type="number" min="0" /></label></div>
   <p>These limits apply to this backup engine, not to desktop or ordinary web transfers.</p>
   <label>Access key ID<input v-model="accessKey" class="input" autocomplete="off" /></label>
   <label>Secret access key<input v-model="secretKey" class="input" type="password" autocomplete="new-password" /></label>
   <label>Optional session token<input v-model="sessionToken" class="input" type="password" autocomplete="new-password" /></label>
   <label><input v-model="clearSessionToken" type="checkbox" /> Clear any previously stored session token</label>
   <label>Repository encryption password<input v-model="password" class="input" type="password" autocomplete="new-password" /></label>
   <p>Blank credential fields retain existing values. Changing endpoint, bucket, or prefix requires re-entering keys and the repository password. Keep a separate offline copy of the repository password; it is not your S3 password.</p>
   <details><summary>Advanced connection settings</summary><label>Custom CA bundle path on server<input v-model.trim="form.caBundle" class="input" /></label><label><input v-model="form.allowHTTP" type="checkbox" /> Allow unencrypted HTTP for trusted local testing only</label><p>No TLS certificate bypass is provided.</p></details>
   <div class="actions"><button class="button" type="submit" :disabled="busy">Save destination</button><button class="button" type="button" :disabled="busy" @click="closeForm">Cancel</button></div>
  </form>
 </div></div>
 <div class="card"><div class="card-title"><h3>Durable recovery catalog</h3></div><div class="card-content">
  <p>This catalog outlives the recent 100-job display. Existing eligible backup records are imported without assuming that their repository snapshots have been freshly verified. A catalog entry is not a substitute for a restore test.</p>
  <p v-if="!catalog.length">No cataloged backups visible in this page for your account and current scope.</p>
  <div v-for="entry in catalog" :key="entry.source.id" class="job"><p>{{ entry.source.createdAt }} — {{ entry.source.destinationId }}</p><p>Snapshot: <code>{{ entry.source.snapshotId }}</code></p>
   <p>Stable retention tag: {{ entry.tagged ? 'recorded for this backup' : 'legacy; no stable retention tag recorded' }}</p>
   <button v-if="canStage(entry.source)" class="button" :disabled="busy || activeJob || !engineAvailable" @click="stageRestore(entry.source)">Restore to private staging</button>
  </div>
  <button v-if="catalogCursor" class="button" :disabled="loadingCatalog" @click="moreCatalog">Load more catalog entries</button>
 </div></div>
 <div class="card"><div class="card-title"><h3>Private staged recoveries</h3></div><div class="card-content">
  <p>Recovery restores only into private server staging. It never replaces live files or the active database, and does not publish a download or share link. Keep this data private: it includes authentication/security state.</p>
  <p>The default limit is 5 GiB per recovery, 50,000 entries and four outstanding recovery records. Free-space guards are best effort, not a filesystem quota. Provider download charges may apply.</p>
  <p>This first version rejects symlinks/special files and accounts with global or user file rules. Backups without scope/export-path metadata require operator-managed recovery; eligible cataloged backups remain accessible after recent-job rollover.</p>
  <p v-if="!restores.length">No staged recoveries recorded for your account and current scope.</p>
  <div v-for="artifact in restores" :key="artifact.id" class="job">
   <h4>{{ artifact.state }}</h4><p>{{ artifact.message }}</p>
   <p>Recovery ID: <code>{{ artifact.id }}</code></p><p>Snapshot: <code>{{ artifact.snapshotId }}</code></p>
   <p>Staged files: {{ artifact.files }}; bytes: {{ artifact.bytes }}; content verified: {{ artifact.contentVerified ? 'yes' : 'no' }}; database header readable: {{ artifact.databaseHeaderReadable ? 'yes' : 'no' }}; staging directory present: {{ artifact.available ? 'yes' : 'no' }}.</p>
   <button class="button" :disabled="busy || activeJob" @click="cleanupRestore(artifact)">Remove staged recovery / record</button>
  </div>
 </div></div>
 <div class="card"><div class="card-title"><h3>Backup and repository jobs</h3></div><div class="card-content">
  <label>Integrity check depth<select v-model.number="integrity" class="input"><option :value="0">Metadata only</option><option :value="1">Read 1% of repository data</option><option :value="100">Read all repository data</option></select></label>
  <p>Data checks may incur provider download charges. A successful data-backup job records its snapshot ID. Still perform integrity checks and restore tests; a snapshot ID alone is not a verified recovery plan.</p>
  <p v-if="!jobs.length">No jobs recorded.</p>
  <div v-for="job in jobs" :key="job.id" class="job">
   <strong>{{ job.action }}: {{ job.state }}</strong><p>{{ job.message }}</p><small>{{ job.createdAt }}; exit code: {{ job.exitCode }}; requested by: {{ job.actor.username || job.actor.userId }}</small>
   <p v-if="job.state === 'queued' || job.state === 'running' || job.state === 'canceling'"><button class="button" :disabled="busy || job.state === 'canceling'" @click="cancel(job.id)">Cancel operation</button></p>
   <p v-if="job.snapshotId">Backup snapshot: <code>{{ job.snapshotId }}</code>; database included: {{ job.databaseIncluded ? 'yes' : 'no' }}; files processed: {{ job.filesProcessed }}; bytes processed: {{ job.bytesProcessed }}</p>
   <p v-if="canStage(job)"><button class="button" :disabled="busy || activeJob || !engineAvailable" @click="stageRestore(job)">Restore this backup to private staging</button></p>
   <ul v-if="job.sources"><li v-for="source in job.sources" :key="source">{{ source }}</li></ul>
   <ul v-if="job.snapshots"><li v-for="snapshot in job.snapshots" :key="snapshot.id"><code>{{ snapshot.id }}</code> {{ snapshot.time }} {{ snapshot.hostname }}</li></ul>
   <p v-if="job.snapshotListTruncated">Snapshot display limited to 50 entries in job history.</p>
  </div>
 </div></div>
</section>
</template>
<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, reactive, ref } from "vue";
import { fetchJSON, fetchURL } from "@/api/utils";
import { useAuthStore } from "@/stores/auth";
interface BackupConfig { sourcePaths?:string[]; sourceOwner?:number; sourceBase?:string; id:string; name:string; enabled:boolean; backend:string; endpoint:string; region:string; bucket:string; prefix:string; addressing:string; connections:number; uploadKiB:number; downloadKiB:number; allowHTTP:boolean; caBundle:string; localRepository?:string; }
interface Destination { config:BackupConfig; secrets:{accessKeySet:boolean;secretKeySet:boolean;sessionTokenSet:boolean;passwordSet:boolean}; }
interface Job { streamId?:string; scheduleId?:string; scheduleWindow?:string; sourceBase?:string; databaseExportPath?:string; restoreArtifactId?:string; sources?:string[]; snapshotId?:string; databaseIncluded?:boolean; filesProcessed?:number; bytesProcessed?:number; id:string;destinationId:string;action:string;state:string;message:string;exitCode:number;createdAt:string;actor:{userId:number;username:string};snapshots?:Array<{id:string;time:string;hostname:string}>;snapshotListTruncated?:boolean; }
interface RestoreArtifact {id:string; sourceJobId:string; snapshotId:string; state:string; message:string; files:number; bytes:number; contentVerified:boolean; databaseHeaderReadable:boolean; available:boolean;}
const authStore=useAuthStore();const restores=ref<RestoreArtifact[]>([]);
interface CatalogEntry {schema:number;tagged:boolean;source:Job;}
const catalog=ref<CatalogEntry[]>([]);const catalogCursor=ref('');const loadingCatalog=ref(false);
const defaults=():BackupConfig=>({sourcePaths:[],id:"",name:"Offsite backups",enabled:false,backend:"s3",endpoint:"",region:"",bucket:"",prefix:"millitop/drive",addressing:"auto",connections:2,uploadKiB:0,downloadKiB:0,allowHTTP:false,caBundle:""});
const form=reactive<BackupConfig>(defaults());const destinations=ref<Destination[]>([]);const jobs=ref<Job[]>([]);const engineAvailable=ref(false);const error=ref("");const notice=ref("");const busy=ref(false);const editing=ref(false);const integrity=ref(0);
const accessKey=ref("");const secretKey=ref("");const sessionToken=ref("");const password=ref("");const clearSessionToken=ref(false);
const activeJob=computed(()=>jobs.value.some(job=>["queued","running","canceling"].includes(job.state)));
let poll:ReturnType<typeof setInterval>|undefined;let refreshing=false;
interface SourceFolder { name:string; path:string; }
const browsePath=ref("/");const currentFolder=ref("/");const sourceFolders=ref<SourceFolder[]>([]);const browsing=ref(false);const canSelectFolder=ref(false);const browseLimited=ref(false);
async function browse(path:string){if(browsing.value)return;browsing.value=true;error.value="";try{const result=await fetchJSON<{path:string;folders:SourceFolder[];canSelect:boolean;limited:boolean}>(`/api/admin/backups/source-folders?path=${encodeURIComponent(path)}`);browsePath.value=result.path;currentFolder.value=result.path;sourceFolders.value=result.folders;canSelectFolder.value=result.canSelect;browseLimited.value=result.limited;}catch(e){error.value=describe(e);}finally{browsing.value=false;}}
function parentFolder(){const parts=currentFolder.value.split("/").filter(Boolean);parts.pop();void browse("/"+parts.join("/"));}
function addSource(path:string){if(path==="/")return;const selected=form.sourcePaths || [];if(selected.some(source=>source===path || path.startsWith(source+"/"))){notice.value="That folder is already covered by the selection.";return;}const next=selected.filter(source=>!source.startsWith(path+"/"));if(next.length>=16){error.value="Select at most 16 folders.";return;}form.sourcePaths=[...next,path];}
function removeSource(path:string){form.sourcePaths=(form.sourcePaths || []).filter(source=>source!==path);}

function describe(e:unknown){return e instanceof Error?e.message:"Request failed.";}
function clearSecrets(){accessKey.value="";secretKey.value="";sessionToken.value="";password.value="";clearSessionToken.value=false;}
function newDestination(){Object.assign(form,defaults());clearSecrets();editing.value=true;error.value="";void browse("/");}
function edit(destination:Destination){Object.assign(form,defaults(),destination.config);clearSecrets();editing.value=true;error.value="";void browse("/");}
function closeForm(){editing.value=false;clearSecrets();}
async function refresh(){if(refreshing)return;refreshing=true;try{const result=await fetchJSON<{destinations:Destination[];engineAvailable:boolean}>("/api/admin/backups/destinations");destinations.value=result.destinations;engineAvailable.value=result.engineAvailable;jobs.value=await fetchJSON<Job[]>("/api/admin/backups/jobs");restores.value=await fetchJSON<RestoreArtifact[]>("/api/admin/backups/restores");if(!loadingCatalog.value){const page=await fetchJSON<{entries:CatalogEntry[];nextCursor?:string}>("/api/admin/backups/catalog");if(catalog.value.length<=50){catalog.value=page.entries;catalogCursor.value=page.nextCursor||"";}else{const known=new Set(catalog.value.map(item=>item.source.id));catalog.value=[...page.entries.filter(item=>!known.has(item.source.id)),...catalog.value];}}}catch(e){error.value=describe(e);}finally{refreshing=false;}}
async function save(){if(busy.value)return;busy.value=true;error.value="";notice.value="";try{const secrets:Record<string,string>={};if(accessKey.value)secrets.accessKeyId=accessKey.value;if(secretKey.value)secrets.secretAccessKey=secretKey.value;if(sessionToken.value)secrets.sessionToken=sessionToken.value;else if(clearSessionToken.value)secrets.sessionToken="";if(password.value)secrets.repositoryPassword=password.value;
 await fetchURL("/api/admin/backups/destinations",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({config:{...form,backend:"s3",localRepository:""},secrets})});closeForm();notice.value="Destination saved. No repository was initialized and no backup job was started.";await refresh();}catch(e){error.value=describe(e);}finally{busy.value=false;}}
async function run(destination:Destination,action:string){if(busy.value)return;let confirmInitialize=false;let recoveryAcknowledged=false;let confirmBackup=false;if(action==="backup"){if(!window.confirm("Create an encrypted backup of the selected Drive folders plus a consistent Drive database export? This writes to your configured repository. File contents are read live, not as an atomic filesystem snapshot."))return;confirmBackup=true;}if(action==="initialize"){if(!window.confirm("Initialize an encrypted repository at this destination? This writes to S3 and may create a bucket if your credentials permit it. Only proceed for the repository you intend to initialize."))return;if(!window.confirm("Have you securely stored the repository encryption password outside this Drive server? You may be unable to restore backups without it."))return;confirmInitialize=true;recoveryAcknowledged=true;}if(action==="check"&&integrity.value>0&&!window.confirm("This check downloads repository data and may incur provider charges. Continue?"))return;
 busy.value=true;error.value="";try{await fetchURL("/api/admin/backups/jobs",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({destinationId:destination.config.id,action,integrityPercent:integrity.value,confirmInitialize,recoveryAcknowledged,confirmBackup})});notice.value="Repository operation queued.";await refresh();}catch(e){error.value=describe(e);}finally{busy.value=false;}}
async function moreCatalog(){if(loadingCatalog.value||!catalogCursor.value)return;loadingCatalog.value=true;try{const page=await fetchJSON<{entries:CatalogEntry[];nextCursor?:string}>(`/api/admin/backups/catalog?cursor=${encodeURIComponent(catalogCursor.value)}`);const seen=new Set(catalog.value.map(item=>item.source.id));catalog.value=[...catalog.value,...page.entries.filter(item=>!seen.has(item.source.id))];catalogCursor.value=page.nextCursor||'';}catch(e){error.value=describe(e);}finally{loadingCatalog.value=false;}}

function canStage(job:Job){const destination=destinations.value.find(item=>item.config.id===job.destinationId);return job.action==='backup' && job.state==='succeeded' && !!job.databaseIncluded && !!job.databaseExportPath && !!job.sourceBase && job.snapshotId?.length===64 && job.actor.userId===authStore.user?.id && destination?.config.enabled===true && destination.config.sourceBase===job.sourceBase;}
async function stageRestore(job:Job){if(busy.value)return;if(!window.confirm("Restore this exact backup into private server staging and verify its content? This downloads repository data and may incur charges. Live files and the live database will NOT be replaced. Staging contains sensitive application data and needs explicit cleanup."))return;busy.value=true;error.value="";try{await fetchURL("/api/admin/backups/jobs",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({destinationId:job.destinationId,action:"restore-staging",restoreFromJobId:job.id,confirmRestore:true})});notice.value="Private staging recovery queued. No live data replacement requested.";await refresh();}catch(e){error.value=describe(e);}finally{busy.value=false;}}
async function cleanupRestore(artifact:RestoreArtifact){if(busy.value)return;if(!window.confirm("Permanently remove this private staged recovery and its registry record? This does NOT delete repository snapshots or live Drive data."))return;busy.value=true;error.value="";try{await fetchURL("/api/admin/backups/restores",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({id:artifact.id,confirmCleanup:true})});notice.value="Staged recovery removed; repository snapshots and live data were not deleted.";await refresh();}catch(e){error.value=describe(e);}finally{busy.value=false;}}

async function cancel(id:string){if(busy.value||!window.confirm("Request cancellation? A repository write may already have occurred."))return;busy.value=true;error.value="";try{await fetchURL(`/api/admin/backups/jobs?id=${encodeURIComponent(id)}`,{method:"DELETE"});await refresh();}catch(e){error.value=describe(e);}finally{busy.value=false;}}
onMounted(()=>{void refresh();poll=setInterval(()=>{void refresh();},5000);});onBeforeUnmount(()=>{if(poll)clearInterval(poll);clearSecrets();});
</script>
<style scoped>
.backup-admin{max-width:1000px;margin:0 auto}.backup-error{color:#c62828;white-space:pre-wrap}.destination,.job{padding:1rem 0;border-bottom:1px solid #8884;overflow-wrap:anywhere}label{display:block;margin:1rem 0 .4rem}input:not([type=checkbox]),select{display:block;width:100%;box-sizing:border-box;margin-top:.4rem}.actions{display:flex;gap:.5rem;flex-wrap:wrap;margin:1rem 0}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:1rem}code{overflow-wrap:anywhere}small{opacity:.8}
</style>
