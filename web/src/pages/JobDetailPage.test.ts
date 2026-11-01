import {it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {Quasar,QLayout,QPageContainer} from 'quasar'
import {createRouter,createMemoryHistory} from 'vue-router'
import JobDetailPage from './JobDetailPage.vue'
import {request} from '../lib/api'
vi.mock('../lib/api',()=>({request:vi.fn()}))
vi.mock('../lib/live',()=>({subscribeEvents:()=>()=>{},liveConnection:{value:'unavailable'}}))
const id='00000000-0000-4000-8000-000000000001'
async function setup(state='running'){
 const job={id,name:'Owned loopback fixture',state,phase:'scanning',config:{targets:['127.0.0.1'],scan_mode:'connect',proxy_version_id:'',ports:'8080'},priority:2,created_at:'2026-10-31T12:00:00Z',started_at:'2026-10-31T12:00:01Z',finished_at:null,error:'',result_count:0,camera_count:0,stream_count:0,inconclusive_count:0,cancel_requested:false,attempt:1,retry_of:null}
 vi.mocked(request).mockImplementation(async(path,opts)=>{if(path.endsWith('/history?limit=200'))return {items:[{seq:'9007199254740993',job_id:id,kind:'phase',created_at:'2026-10-31T12:00:01Z',data:{phase:'scanning',message:'Fixture scanner started'}}],has_more:false} as never;if(opts?.method==='POST')return {...job,state:'canceling',cancel_requested:true} as never;return job as never})
 const router=createRouter({history:createMemoryHistory(),routes:[{path:'/:pathMatch(.*)*',component:{template:'<div />'}}]});await router.push('/jobs/'+id);await router.isReady()
 const w=mount({components:{JobDetailPage,QLayout,QPageContainer},setup:()=>({id}),template:'<q-layout><q-page-container><JobDetailPage :id="id"/></q-page-container></q-layout>'},{attachTo:document.body,global:{plugins:[router,[Quasar,{}]]}});await flushPromises();return w
}
it('requires explicit authorization before retrying a terminal job',async()=>{const w=await setup('orphaned');await w.get('[data-testid="retry-job"]').trigger('click');await flushPromises();expect(document.body.textContent).toContain('Run this scan again with the same settings');const button=document.querySelector<HTMLButtonElement>('[data-testid="confirm-retry"]');expect(button).not.toBeNull();expect(button!.disabled).toBe(true);document.querySelector<HTMLElement>('[data-testid="authorize-retry"]')!.click();await flushPromises();button!.click();await flushPromises();expect(request).toHaveBeenCalledWith('/jobs/'+id+'/retry',expect.objectContaining({method:'POST',body:{idempotency_key:expect.any(String)}}));w.unmount()})
it('opens the durable job record and its stored event timeline',async()=>{const w=await setup();expect(w.text()).toContain('Owned loopback fixture');expect(w.text()).toContain('Fixture scanner started');expect(w.text()).toContain('127.0.0.1');expect(w.find('video').exists()).toBe(false);w.unmount()})
it('requires confirmation before requesting cancellation',async()=>{const w=await setup();await w.get('[data-testid="cancel-job"]').trigger('click');await flushPromises();expect(vi.mocked(request).mock.calls.some(([,o])=>o?.method==='POST')).toBe(false);const button=document.querySelector<HTMLButtonElement>('[data-testid="confirm-cancel"]');expect(button).not.toBeNull();button!.click();await flushPromises();expect(request).toHaveBeenCalledWith('/jobs/'+id+'/cancel',expect.objectContaining({method:'POST'}));w.unmount()})
