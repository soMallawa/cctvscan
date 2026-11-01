<script setup lang="ts">
import {ref,computed,onMounted,onBeforeUnmount} from 'vue'
import {useRouter,useRoute} from 'vue-router'
import {matAdd,matRefresh,matSearch,matClose,matStop,matArrowForward} from '@quasar/extras/material-icons'
import {request} from '../lib/api'
import {terminalStates,type Job,type Page,type Overview} from '../types'
const router=useRouter(),route=useRoute()
const rows=ref<Job[]>([]),total=ref<number|null>(null),loading=ref(true),error=ref(''),search=ref(''),state=ref(''),offset=ref(0),pageSize=ref(10)
const counts=ref<Record<string,number>>({}),selectedID=ref(typeof route.query.job==='string'?route.query.job:''),pending=ref(false),cancelDialog=ref(false)
const selected=computed(()=>rows.value.find(j=>j.id===selectedID.value))
const states=['queued','running','canceling','completed','inconclusive','failed','canceled','timed_out','orphaned']
let controller:AbortController|undefined
function title(s:string){return s.replaceAll('_',' ').replace(/^./,c=>c.toUpperCase())}
function date(s:string|null){return s?new Date(s).toLocaleString(undefined,{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}):'Not started'}
function select(job:Job){selectedID.value=job.id;void router.replace({query:{...route.query,job:job.id}})}
async function load(){
 controller?.abort();controller=new AbortController();const signal=controller.signal;loading.value=true;error.value=''
 try{
  const page=await request<Page<Job>>(`/jobs?limit=${pageSize.value}&offset=${offset.value}&search=${encodeURIComponent(search.value)}&state=${encodeURIComponent(state.value)}`,{signal})
  if(signal.aborted)return;rows.value=page.items;total.value=page.total
 }catch(e){if(!signal.aborted)error.value=e instanceof Error?e.message:'Could not load jobs. Try again.'}
 finally{if(!signal.aborted)loading.value=false}
}
async function refresh(){await load();try{const overview=await request<Overview>('/overview?days=7');counts.value=overview.counts??{}}catch{counts.value={}}}
function filter(){offset.value=0;void load()}
async function cancel(){if(!selected.value)return;pending.value=true;try{await request(`/jobs/${encodeURIComponent(selected.value.id)}/cancel`,{method:'POST',body:{}});cancelDialog.value=false;await refresh()}catch(e){error.value=e instanceof Error?e.message:'Cancellation could not be requested.'}finally{pending.value=false}}
onMounted(refresh);onBeforeUnmount(()=>controller?.abort())
</script>
<template>
 <q-page class="workspace">
  <header class="page-header"><div><p class="page-kicker">Operations</p><h1>Job dispatch</h1><p class="page-description">From defined scope to inspectable evidence. Every job has a durable record.</p></div><q-btn data-testid="new-scan" unelevated color="primary" text-color="dark" :icon="matAdd" label="New scan" @click="router.push('/jobs/new')"/></header>
  <div class="dispatch-summary"><span><strong>{{counts.queued??'—'}}</strong> queued</span><span><strong>{{counts.running??'—'}}</strong> running</span><span><strong>{{counts.completed??'—'}}</strong> completed</span><span class="summary-note">Manual refresh</span></div>
  <div v-if="error" class="error-banner" role="alert"><span>{{error}}</span><q-btn flat label="Try again" @click="refresh"/></div>
  <div class="dispatch-layout" :class="{'has-selection':selected}">
   <section class="dispatch-board" aria-label="Job queue" :aria-busy="loading">
    <div class="board-toolbar"><div class="board-title">Queue <span>{{total===null?'Not loaded':`${total} total`}}</span></div><q-btn flat round dense :icon="matRefresh" aria-label="Refresh jobs" :loading="loading" @click="refresh"/></div>
    <div class="board-filters"><q-input v-model="search" dark dense outlined debounce="300" clearable aria-label="Search jobs" placeholder="Search name or scope" @update:model-value="filter"><template #prepend><q-icon :name="matSearch" size="18px"/></template></q-input><q-select v-model="state" dark dense outlined emit-value map-options :options="[{label:'All states',value:''},...states.map(s=>({label:title(s),value:s}))]" aria-label="Filter by state" @update:model-value="filter"/></div>
    <div v-if="loading&&!rows.length" class="board-loading" role="status">Loading the queue…<q-linear-progress indeterminate color="primary"/></div>
    <div v-else-if="!rows.length&&!error" class="board-empty"><span class="empty-marker" aria-hidden="true">/</span><h2>{{search||state?'No matching jobs':'No jobs yet'}}</h2><p>{{search||state?'Adjust the filters to find a job.':'Define an authorized scope, review the plan, then send it to the queue.'}}</p><q-btn v-if="!search&&!state" flat color="primary" label="Create the first scan" @click="router.push('/jobs/new')"/></div>
    <div v-else-if="error&&!rows.length" class="board-empty"><h2>Queue unavailable</h2><p>The saved queue could not be loaded. Retry the connection; this is not a verified empty queue.</p></div>
    <div v-else class="board-table-scroll"><table class="jobs-table"><thead><tr><th>Job / scope</th><th>State</th><th>Evidence</th><th>Created</th></tr></thead><tbody><tr v-for="job in rows" :key="job.id" :class="{selected:job.id===selectedID}" @click="select(job)"><td><button class="job-name" :aria-pressed="job.id===selectedID" @click.stop="select(job)">{{job.name}}</button><span class="job-scope">{{job.config.targets.join(', ')}}</span></td><td><span class="status-badge" :data-state="job.state">{{title(job.state)}}</span><span class="job-phase">{{job.phase?title(job.phase):'Phase not reported'}}</span></td><td class="numeric">{{job.result_count}} <small>hosts</small></td><td class="job-time">{{date(job.created_at)}}</td></tr></tbody></table></div>
    <footer class="board-pagination compact-pagination" aria-label="Pagination"><label class="page-size-label">Rows <select v-model.number="pageSize" @change="filter" :disabled="loading" aria-label="Rows per page"><option :value="10">10</option><option :value="25">25</option><option :value="50">50</option></select></label><span>{{total===null?'Loading…':`${total?offset+1:0}–${Math.min(offset+pageSize,total)} of ${total}`}}</span><div><q-btn flat dense label="Previous" :disable="loading||offset===0" @click="offset=Math.max(0,offset-pageSize);load()"/><q-btn flat dense label="Next" :disable="loading||total===null||offset+pageSize>=total" @click="offset+=pageSize;load()"/></div></footer>
   </section>
   <aside v-if="selected" class="job-inspector" aria-label="Job inspector">
    <template v-if="selected"><div class="inspector-top"><span class="section-label">Job inspector</span><q-btn flat round dense :icon="matClose" aria-label="Close inspector" @click="selectedID='';router.replace({query:{...route.query,job:undefined}})"/></div><span class="status-badge" :data-state="selected.state">{{title(selected.state)}}</span><h2>{{selected.name}}</h2><p class="job-id">{{selected.id}}</p><dl class="detail-list"><div><dt>Requested scope</dt><dd>{{selected.config.targets.join(', ')}}</dd></div><div><dt>Current phase</dt><dd>{{selected.phase?title(selected.phase):'Not reported'}}</dd></div><div><dt>Started</dt><dd>{{date(selected.started_at)}}</dd></div><div><dt>Finished</dt><dd>{{selected.finished_at?date(selected.finished_at):'Not finished'}}</dd></div><div><dt>Attempt</dt><dd>{{selected.attempt}}</dd></div><div><dt>HTTP proxy</dt><dd>{{selected.config.proxy_version_id?'Pinned proxy version':'Direct HTTP requests'}}</dd></div></dl><div class="inspector-counts"><span><strong>{{selected.result_count}}</strong>Hosts</span><span><strong>{{selected.camera_count}}</strong>Cameras</span><span><strong>{{selected.stream_count}}</strong>Stream evidence</span></div><p v-if="selected.error" class="inline-error">{{selected.error}}</p><q-btn unelevated color="dark" class="full-width" :icon-right="matArrowForward" label="Open job record" @click="router.push(`/jobs/${selected.id}`)"/><q-btn v-if="!terminalStates.has(selected.state)" flat class="full-width q-mt-sm" color="negative" :icon="matStop" :disable="selected.cancel_requested" :label="selected.cancel_requested?'Cancellation requested':'Cancel job'" @click="cancelDialog=true"/></template>
    <div v-else class="inspector-empty"><span class="section-label">Job inspector</span><h2>The details,<br>not just the status.</h2><p>Select a job to inspect its scope, execution and evidence.</p><div class="inspector-rule"/><p class="micro-copy">No job runs in the browser. Work stays in the queue when this page closes.</p></div>
   </aside>
  </div>
  <q-dialog v-model="cancelDialog"><q-card class="confirmation-dialog"><q-card-section><h2>Cancel this job?</h2><p>The runner will stop processing and preserve evidence already collected. In-flight network work may take a short time to stop.</p></q-card-section><q-card-actions align="right"><q-btn flat label="Keep running" v-close-popup/><q-btn unelevated color="negative" label="Request cancellation" :loading="pending" @click="cancel"/></q-card-actions></q-card></q-dialog>
 </q-page>
</template>
