<script setup lang="ts">
import {computed,ref,watch,onBeforeUnmount} from 'vue'
import {request} from '../lib/api'
import {RouterLink} from 'vue-router'
type Row=Record<string,unknown>
interface Assessment {state:string;reason:string;brand?:string;candidates:{id:string;model:string;brand:string}[];revision?:string}
const props=defineProps<{resultId:string;result:Row}>(),assessment=ref<Assessment|null>(null),error=ref('')
let controller:AbortController|undefined
const obj=(v:unknown):Row=>v&&typeof v==='object'?v as Row:{}
const checks=computed(()=>Array.isArray(props.result.AutomaticVerification)?props.result.AutomaticVerification.map(v=>{const check=obj(v);return {profile_id:check.profile_id,attempted_pairs:check.attempted_pairs,credential_vendor:check.credential_vendor,outcome:obj(check.outcome)}}):[])
const label=(v:unknown)=>String(v??'Not recorded').replace(/_/g,' ').replace(/^./,s=>s.toUpperCase())
function preview(outcome:Row){const b=outcome.frame_base64;return typeof b==='string'&&b.length<=350000&&/^[A-Za-z0-9+/]+=*$/.test(b)?'data:image/jpeg;base64,'+b:''}
watch(()=>props.resultId,async(id)=>{controller?.abort();const c=new AbortController();controller=c;assessment.value=null;error.value='';try{const d=await request<{assessment:Assessment}>(`/results/${encodeURIComponent(id)}/catalogue`,{signal:c.signal});if(!c.signal.aborted)assessment.value=d.assessment}catch{if(!c.signal.aborted)error.value='Reference matching unavailable. Stored scan outcomes are still shown.'}},{immediate:true})
onBeforeUnmount(()=>controller?.abort())
</script>
<template>
 <section class="automatic-verification" aria-label="Automatic camera verification">
  <h3>Automatic camera verification</h3><p class="helper">Uses the scan’s saved vendor credentials, selected ports and route. No per-device setup.</p>
  <p v-if="error" class="helper">{{error}}</p>
  <div v-if="assessment" class="reference-assessment"><strong>{{label(assessment.state)}}</strong><p class="helper">{{assessment.reason}}</p><ul v-if="assessment.candidates.length"><li v-for="m in assessment.candidates" :key="m.id"><router-link :to="{path:'/catalogue',query:{search:m.model}}">{{m.brand}} {{m.model}} ↗</router-link></li></ul></div>
  <p v-if="!checks.length" class="helper">No automatic connection outcome was retained. Historical results are not rescanned when opened.</p>
  <article v-for="(check,index) in checks" :key="resultId+':'+index" class="verification-check">
   <header><strong>{{label(check.outcome.state)}}</strong><span>{{label(check.outcome.route)}}</span></header>
   <dl><dt>Authentication</dt><dd>{{label(check.outcome.authentication)}}<template v-if="check.outcome.auth_method"> · {{check.outcome.auth_method}}</template></dd><dt>Media</dt><dd>{{label(check.outcome.media)}}</dd><template v-if="check.outcome.width"><dt>Decoded dimensions</dt><dd>{{check.outcome.width}} × {{check.outcome.height}}</dd></template><dt>Reference</dt><dd>{{check.profile_id}}</dd><template v-if="check.attempted_pairs"><dt>Credential checks</dt><dd>{{check.attempted_pairs}} · {{check.credential_vendor||'Generic'}} first</dd></template></dl>
   <p class="helper">{{check.outcome.reason}}</p><img v-if="preview(check.outcome)" :src="preview(check.outcome)" alt="Frame captured and decoded during this scan" loading="lazy"/><p v-else-if="check.outcome.state==='frame_verified'" class="helper">Frame decoded; preview was not retained under this scan’s capture policy.</p>
  </article>
 </section>
</template>
<style scoped>
.automatic-verification{margin-top:22px;border-top:1px solid var(--line);padding-top:18px}.automatic-verification h3{font-size:14px;margin:0}.reference-assessment{margin:14px 0;font-size:12px}.reference-assessment ul{padding-left:18px}.reference-assessment a{color:var(--ink);font-weight:600}.verification-check{border:1px solid var(--line);padding:12px;border-radius:6px;margin-top:12px;overflow-wrap:anywhere}.verification-check header{display:flex;justify-content:space-between;gap:8px;font-size:12px}.verification-check header span{font-size:10px;color:var(--muted)}.verification-check dl{display:grid;grid-template-columns:105px minmax(0,1fr);gap:7px;font-size:11px;margin:14px 0}.verification-check dt{color:var(--muted)}.verification-check dd{margin:0}.verification-check img{display:block;width:100%;height:auto;max-height:300px;object-fit:contain;background:var(--canvas);border-radius:4px}.verification-check .helper{font-size:11px}
</style>
