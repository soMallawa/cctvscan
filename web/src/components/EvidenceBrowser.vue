<script setup lang="ts">
import {computed,ref,onMounted,onBeforeUnmount,watch} from 'vue'
import {matRefresh,matSearch,matClose,matArrowForward} from '@quasar/extras/material-icons'
import {request} from '../lib/api'
import type {Page} from '../types'
import ConnectionWorkbench from './ConnectionWorkbench.vue'
interface StoredResult {id:string;job_id:string;target:string;brand:string;state:string;result:Record<string,unknown>}
const props=withDefaults(defineProps<{jobId?:string}>(),{jobId:''}),rows=ref<StoredResult[]>([]),total=ref<number|null>(null),selected=ref<StoredResult|null>(null),search=ref(''),state=ref(''),offset=ref(0),pageSize=ref(10),loading=ref(true),error=ref('')
let controller:AbortController|undefined,timer:ReturnType<typeof setInterval>|undefined
async function load(){controller?.abort();controller=new AbortController();const signal=controller.signal;loading.value=true;error.value='';try{const p=await request<Page<StoredResult>>(`/results?limit=${pageSize.value}&offset=${offset.value}&job_id=${encodeURIComponent(props.jobId)}&search=${encodeURIComponent(search.value)}&state=${encodeURIComponent(state.value)}`,{signal});if(signal.aborted)return;rows.value=p.items;total.value=p.total;if(selected.value)selected.value=p.items.find(r=>r.id===selected.value?.id)??selected.value}catch(e){if(!signal.aborted)error.value=e instanceof Error?e.message:'Stored evidence could not be loaded.'}finally{if(!signal.aborted)loading.value=false}}
function filter(){offset.value=0;void load()}
function setSearch(v:string|number|null){search.value=String(v??'');filter()}
type RecordValue = Record<string,unknown>
function record(value:unknown):RecordValue{return value&&typeof value==='object'&&!Array.isArray(value)?value as RecordValue:{}}
function camera(r:StoredResult){return record(r.result.CameraDetection)}
function detectionLabel(d:RecordValue){
 if(d.State==='supported')return 'Supported'
 if(d.State==='possible')return 'Possible'
 if(d.State==='unknown')return 'Unknown'
 return d.IsCamera===true?'Legacy positive — unverified':'Unknown (legacy)'
}
function detections(r:StoredResult){return Array.isArray(r.result.CameraDetections)?r.result.CameraDetections.map(record):[]}
function decodeBody(e:RecordValue){
 const value=e.BytesBase64
 if(typeof value!=='string'||!value)return {text:'',message:'No body retained.'}
 if(value.length>11000)return {text:'',message:'Stored body exceeds the preview limit.'}
 if(!/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value))return {text:'',message:'Invalid base64 body.'}
 try{
  const raw=atob(value)
  if(raw.length>8192)return {text:'',message:'Stored body exceeds the preview limit.'}
  return {text:new TextDecoder().decode(Uint8Array.from(raw,c=>c.charCodeAt(0))),message:''}
 }catch{return {text:'',message:'Invalid base64 body.'}}
}
const endpoints=computed(()=>{
 const values=record(selected.value?.result.HTTPMeta).Endpoints
 return Array.isArray(values)?values.map(value=>{const endpoint=record(value);const evidence=record(endpoint.RootEvidence);return {endpoint,evidence,body:decodeBody(evidence)}}):[]
})
function downloadBody(text:string){
 const url=URL.createObjectURL(new Blob([text],{type:'text/plain;charset=utf-8'}))
 const link=document.createElement('a');link.href=url;link.download='root-response.txt'
 try{link.click()}finally{URL.revokeObjectURL(url)}
}
function ports(r:StoredResult){const p=r.result?.ports??r.result?.Ports;return Array.isArray(p)?p.filter(v=>typeof v==='number').join(', ')||'None reported':'Not reported'}
watch(()=>props.jobId,()=>{offset.value=0;rows.value=[];total.value=null;selected.value=null;void load()});onMounted(()=>{void load();timer=setInterval(()=>{if(!loading.value)void load()},15000)});onBeforeUnmount(()=>{controller?.abort();clearInterval(timer)})
defineExpose({refresh:load})
</script>
<template>
 <div class="evidence-browser"><div v-if="error" role="alert" class="error-banner"><span>{{error}}<template v-if="rows.length"> Showing the last loaded results.</template></span><q-btn flat label="Try again" @click="load"/></div><div class="evidence-layout" :class="{'has-selection':selected}"><section class="surface evidence-results" aria-label="Stored host results" :aria-busy="loading"><header class="section-heading"><h2>Host results</h2><div class="row items-center q-gutter-sm"><span class="helper">{{total===null?'Not loaded':`${total} total`}}</span><q-btn flat round dense :icon="matRefresh" :loading="loading" aria-label="Refresh stored evidence" @click="load"/></div></header><div class="evidence-filters"><q-input :model-value="search" @update:model-value="setSearch" outlined dense clearable debounce="300" aria-label="Search stored targets or brands" placeholder="Search target or brand"><template #prepend><q-icon :name="matSearch"/></template></q-input><q-select v-model="state" @update:model-value="filter" outlined dense emit-value map-options aria-label="Filter evidence outcome" :options="[{label:'All outcomes',value:''},...['completed','inconclusive','canceled','failed'].map(v=>({label:v,value:v}))]"/></div><p v-if="loading&&!rows.length" class="loading-label" role="status">Loading stored evidence…</p><div v-else-if="!rows.length&&!error" class="resource-empty"><h2>No stored evidence</h2><p>Results will appear here after a scan.</p></div><div v-else class="resource-table-scroll"><table class="resource-table"><thead><tr><th>Target</th><th>Brand</th><th>Host outcome</th><th>Detection</th><th>Open ports</th></tr></thead><tbody><tr v-for="r in rows" :key="r.id" :class="{'selected-evidence':selected?.id===r.id}"><td><button data-testid="inspect-result" class="job-name result-target" :aria-pressed="selected?.id===r.id" @click="selected=r">{{r.target}}</button></td><td>{{r.brand||'Not identified'}}</td><td><span class="status-badge" :data-state="r.state">{{r.state.replaceAll('_',' ')}}</span></td><td data-testid="detection-label">{{detectionLabel(camera(r))}}</td><td class="mono result-ports">{{ports(r)}}</td></tr></tbody></table></div><footer class="compact-pagination" aria-label="Pagination"><label class="page-size-label">Rows <select v-model.number="pageSize" @change="filter" :disabled="loading" aria-label="Rows per page"><option :value="10">10</option><option :value="25">25</option><option :value="50">50</option></select></label><span>{{total===null?'Loading…':`${total?offset+1:0}–${Math.min(offset+pageSize,total)} of ${total}`}}</span><div><q-btn flat dense label="Previous" :disable="loading||offset===0" @click="offset=Math.max(0,offset-pageSize);load()"/><q-btn flat dense label="Next" :disable="loading||total===null||offset+pageSize>=total" @click="offset+=pageSize;load()"/></div></footer></section><aside v-if="selected" class="surface result-inspector" aria-label="Evidence inspector"><div class="inspector-heading"><span class="section-label">Stored evidence</span><q-btn flat round dense :icon="matClose" aria-label="Close evidence inspector" @click="selected=null"/></div><h2>{{selected.target}}</h2><span class="status-badge q-mt-md" :data-state="selected.state">{{selected.state.replaceAll('_',' ')}}</span><q-btn flat dense class="q-mt-md" :icon-right="matArrowForward" label="Source job" :to="'/jobs/'+encodeURIComponent(selected.job_id)"/><div class="inspector-rule"/><section data-testid="camera-triage"><h3>Camera detection · {{detectionLabel(camera(selected))}}</h3><p class="helper q-mt-sm">{{camera(selected).Reason || 'No detection reason retained.'}}</p><ul v-if="detections(selected).length" class="detection-list"><li v-for="(d,index) in detections(selected)" :key="index">{{detectionLabel(d)}} · {{d.Reason || 'No detection reason retained.'}}</li></ul></section><section class="http-evidence" aria-label="Stored HTTP endpoints">
 <ConnectionWorkbench :result-id="selected.id" :result="selected.result"/>
 <h3>HTTP endpoints</h3>
 <p class="helper">Stored responses only · up to 8 KiB per root, 32 KiB per host. No device requests.</p>
 <p v-if="!endpoints.length" class="helper">No endpoint evidence retained for this result.</p>
 <details v-for="({endpoint:e,evidence,body},index) in endpoints" :key="selected.id+':'+index" class="endpoint-evidence" data-testid="http-endpoint">
  <summary>{{e.StatusCode || 'No status'}} · {{e.URL || 'Endpoint'}} <span class="helper">{{e.State}}</span></summary>
  <dl class="endpoint-meta"><dt>Server</dt><dd>{{e.Server || 'Not reported'}}</dd><dt>Type</dt><dd>{{e.ContentType || 'Not reported'}}</dd><template v-if="e.FinalURL"><dt>Final URL</dt><dd>{{e.FinalURL}}</dd></template></dl>
  <p v-if="e.Error" class="helper">{{e.Error}}</p>
  <p v-if="!Object.keys(evidence).length" class="helper">Root response not retained for this endpoint (or legacy result).</p>
  <template v-else>
   <p class="helper">{{evidence.Authenticated===false?'Unauthenticated':'Authentication not confirmed'}} {{evidence.Method || 'Method not reported'}} · {{evidence.CapturedAt || 'Capture time not reported'}}</p>
   <p class="helper">{{evidence.CapturedBytes ?? 'Unknown'}} bytes / {{evidence.Limit ?? 8192}} byte limit · {{evidence.Truncated?'Truncated':'Not truncated'}}</p>
   <dl class="endpoint-meta"><dt>SHA256</dt><dd class="mono">{{evidence.SHA256 || 'Not retained'}}</dd><template v-if="evidence.WWWAuthenticate"><dt>Auth challenge</dt><dd>{{evidence.WWWAuthenticate}}</dd></template></dl>
   <p v-if="body.message" class="helper">{{body.message}}</p>
   <template v-else><pre class="evidence-body" tabindex="0" aria-label="Stored root response body">{{body.text}}</pre><q-btn flat dense label="Download .txt" @click="downloadBody(body.text)"/></template>
  </template>
 </details>
 </section><details data-testid="raw-evidence" class="raw-evidence" :key="selected.id"><summary>Raw JSON · sanitized result record</summary><p class="helper q-mt-sm">Authentication and query strings may be removed. No browser request is made to device URLs.</p><pre class="evidence-json">{{JSON.stringify(selected.result,(key,value)=>key==='frame_base64'?'[decoded preview above]':value,2)}}</pre></details></aside></div></div>
</template>
<style scoped>
.http-evidence,.raw-evidence{margin-top:20px}.endpoint-evidence{border-top:1px solid var(--line);padding:10px 0;font-size:12px;overflow-wrap:anywhere}.endpoint-evidence summary,.raw-evidence summary{cursor:pointer;font-weight:650;line-height:1.5;padding:4px 0}.endpoint-meta{display:grid;grid-template-columns:auto minmax(0,1fr);gap:4px 10px;font-size:11px;margin:10px 0}.endpoint-meta dt{font-weight:650}.endpoint-meta dd{margin:0;overflow-wrap:anywhere}.evidence-body{font-size:11px;line-height:1.6;white-space:pre-wrap;overflow-wrap:anywhere;max-height:280px;overflow:auto;padding:12px;background:var(--canvas);border-radius:6px}.detection-list{font-size:12px;padding-left:18px}summary:focus-visible,.evidence-body:focus-visible{outline:2px solid var(--ink);outline-offset:3px}
.evidence-results{min-width:0}.evidence-layout{display:grid;grid-template-columns:minmax(0,1fr);gap:20px;align-items:start}.evidence-layout.has-selection{grid-template-columns:minmax(0,1fr) 360px}.section-heading h2{font-size:18px}.evidence-filters{display:grid;grid-template-columns:minmax(0,1fr) 170px;gap:12px;padding:18px 24px;border-bottom:1px solid var(--line)}.selected-evidence{background:#f1f3f5}.result-target{color:var(--ink);font-weight:650;font-size:13px}.result-target:hover{color:#6b5511}.result-ports{font-size:11px;max-width:230px;overflow-wrap:anywhere}.result-inspector{padding:22px;position:sticky;top:24px;min-width:0}.result-inspector h2{font-size:22px;margin-top:20px;overflow-wrap:anywhere}.evidence-json{font-size:11px;line-height:1.7;margin:18px -8px 0;padding:14px;background:var(--canvas);overflow:auto;max-height:540px;white-space:pre-wrap;overflow-wrap:anywhere;border-radius:6px}.result-inspector h3{font-size:14px;margin:0}.resource-table th{white-space:nowrap}@media(max-width:1250px){.evidence-layout.has-selection{grid-template-columns:minmax(0,1fr)}.result-inspector{position:static}.evidence-json{max-height:380px}}@media(max-width:600px){.evidence-filters{grid-template-columns:minmax(0,1fr);padding:16px}.result-inspector{padding:20px}}</style>
