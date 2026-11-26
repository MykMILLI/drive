<template>
 <section class="automation">
  <div class="card"><div class="card-title"><h2>Backup scheduling</h2></div><div class="card-content">
   <p>Development integration: scheduling and durable recovery catalog. Retention deletion, maintenance replacement, Proxmox delivery and broader audit are not completed by this batch. Do not deploy this batch as the integrated release.</p>
   <p v-if="error" class="error" role="alert">{{ error }}</p><p v-if="notice" role="status">{{ notice }}</p>
   <p>Schedules use the displayed IANA time zone. Empty weekdays means every day. Nonexistent daylight-saving times are skipped; a repeated wall-clock hour does not create two executions for the same local-date window.</p>
   <p>Overdue days collapse to one pending execution. Busy backup operations defer admission. Uncertain/failed writes are not automatically replayed. Disabling a schedule does not cancel a job already started.</p>
   <div class="actions"><button class="button" :disabled="busy" @click="create">New schedule</button><button class="button" :disabled="busy" @click="refresh">Refresh</button></div>
   <div v-for="item in schedules" :key="item.id" class="item"><h3>{{ item.name }} — {{ item.enabled ? 'enabled' : 'disabled' }}</h3>
    <p>{{ item.state }}: {{ item.message }}</p><p>Next: {{ item.nextRunAt }} (UTC); zone: {{ item.timeZone }}; last job: {{ item.lastJobId || 'none' }}</p>
    <button class="button" :disabled="busy" @click="edit(item)">Edit</button>
   </div>
   <p v-if="!schedules.length">No schedules saved. Nothing is enabled automatically.</p>
  </div></div>
  <div v-if="editing" class="card"><div class="card-title"><h3>{{ form.id ? 'Edit schedule' : 'New schedule' }}</h3></div><div class="card-content">
   <form @submit.prevent="save">
    <label>Name<input v-model.trim="form.name" class="input" maxlength="100" required /></label>
    <label>Destination<select v-model="form.destinationId" class="input" required><option value="">Choose destination</option><option v-for="item in destinations" :key="item.config.id" :value="item.config.id">{{ item.config.name }}</option></select></label>
    <label><input v-model="form.enabled" type="checkbox" /> Enable this schedule</label>
    <label>IANA time zone<input v-model.trim="form.timeZone" class="input" placeholder="UTC or America/New_York" required /></label>
    <label>Local hour<input v-model.number="form.hour" class="input" type="number" min="0" max="23" required /></label>
    <label>Local minute<input v-model.number="form.minute" class="input" type="number" min="0" max="59" required /></label>
    <fieldset><legend>Optional weekdays; none selected means every day</legend><label v-for="(name,index) in days" :key="name"><input v-model="form.weekdays" type="checkbox" :value="index" /> {{ name }}</label></fieldset>
    <div class="actions"><button class="button" :disabled="busy" type="submit">Save schedule</button><button class="button" :disabled="busy" type="button" @click="editing=false">Cancel</button></div>
   </form>
  </div></div>
 </section>
</template>
<script setup lang="ts">
import {ref,reactive,onMounted,onBeforeUnmount} from 'vue';
import {fetchJSON,fetchURL} from '@/api/utils';
interface Input {id:string;version:number;name:string;destinationId:string;enabled:boolean;timeZone:string;hour:number;minute:number;weekdays:number[];}
interface Schedule extends Input {state:string;message:string;nextRunAt:string;lastJobId:string;}
interface Destination {config:{id:string;name:string;};}
const schedules=ref<Schedule[]>([]);const destinations=ref<Destination[]>([]);const error=ref('');const notice=ref('');const busy=ref(false);const editing=ref(false);const days=['Sunday','Monday','Tuesday','Wednesday','Thursday','Friday','Saturday'];
const defaults=():Input=>({id:'',version:0,name:'Nightly backup',destinationId:'',enabled:false,timeZone:'UTC',hour:2,minute:0,weekdays:[]});
const form=reactive<Input>(defaults());let poll:ReturnType<typeof setInterval>|undefined;let refreshing=false;
function create(){Object.assign(form,defaults());editing.value=true;}
function edit(item:Schedule){Object.assign(form,{id:item.id,version:item.version,name:item.name,destinationId:item.destinationId,enabled:item.enabled,timeZone:item.timeZone,hour:item.hour,minute:item.minute,weekdays:[...(item.weekdays||[])]});editing.value=true;}
function describe(e:unknown){return e instanceof Error?e.message:'Request failed';}
async function refresh(){if(refreshing)return;refreshing=true;try{schedules.value=await fetchJSON<Schedule[]>('/api/admin/backups/schedules');const result=await fetchJSON<{destinations:Destination[]}>('/api/admin/backups/destinations');destinations.value=result.destinations;}catch(e){error.value=describe(e);}finally{refreshing=false;}}
async function save(){if(busy.value)return;if(form.enabled&&!window.confirm('Enable automated encrypted backups for this destination and selected folders? Repository writes and provider charges may occur at the scheduled time.'))return;busy.value=true;error.value='';try{await fetchURL('/api/admin/backups/schedules',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({...form})});editing.value=false;notice.value='Schedule saved. Disabling does not cancel an existing job.';await refresh();}catch(e){error.value=describe(e);}finally{busy.value=false;}}
onMounted(()=>{void refresh();poll=setInterval(()=>void refresh(),15000);});onBeforeUnmount(()=>{if(poll)clearInterval(poll);});
</script>
<style scoped>.automation{max-width:1000px;margin:0 auto}.error{color:#c62828;white-space:pre-wrap}.item{padding:1rem 0;border-bottom:1px solid #8884;overflow-wrap:anywhere}label{display:block;margin:1rem 0}.input{display:block;width:100%;box-sizing:border-box}.actions{display:flex;gap:.5rem;flex-wrap:wrap;margin:1rem 0}</style>
