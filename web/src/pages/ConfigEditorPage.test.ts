import {it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {Quasar,QLayout,QPageContainer} from 'quasar'
import {createRouter,createMemoryHistory} from 'vue-router'
import ConfigEditorPage from './ConfigEditorPage.vue'
import {request} from '../lib/api'
vi.mock('../lib/api',()=>({request:vi.fn()}))
const config={targets:['127.0.0.1'],port_mode:'custom',ports:'8080',scan_mode:'connect',rate:100,retry:0,wait:1,threads:1,adapter:'',adapter_ip:'',router_ip:'',router_mac:'',route_target:'',timeout_seconds:30,host_timeout_seconds:5,host_workers:1,http_requests:1,http_per_host:1,http_rps:1,max_targets:1,proxy_version_id:'',credential_set_id:'',check_credentials:false,capture:false,onvif:false,debug:false}
it.each(['profiles','schedules'] as const)('saves a complete %s configuration without dispatching a scan',async(kind)=>{
 vi.mocked(request).mockImplementation(async(path,options)=>{
  if(path==='/bootstrap')return {defaults:config,policy:{allow_raw:false,allow_credentials:true,allow_capture:true,allow_onvif:true},auth_mode:'local',version:'test'} as never
  if(options?.method==='POST')return {id:'owned-record',name:'Owned configuration',config} as never
  return {items:[],total:0} as never
 })
 const router=createRouter({history:createMemoryHistory(),routes:[{path:'/:pathMatch(.*)*',component:{template:'<div />'}}]});await router.push(`/${kind}/new`);await router.isReady()
 const w=mount({components:{ConfigEditorPage,QLayout,QPageContainer},setup:()=>({kind}),template:'<q-layout><q-page-container><ConfigEditorPage :kind="kind"/></q-page-container></q-layout>'},{attachTo:document.body,global:{plugins:[router,[Quasar,{}]]}});await flushPromises()
 expect(w.find('[data-testid="save-configuration"]').exists()).toBe(true)
 await w.get('input[name="configuration_name"]').setValue('Owned configuration');await w.get('[data-testid="save-configuration"]').trigger('click');await flushPromises()
 expect(request).toHaveBeenCalledWith(`/${kind}`,expect.objectContaining({method:'POST',body:expect.objectContaining({name:'Owned configuration',config,...(kind==='schedules'?{enabled:false,misfire_policy:'skip',overlap_policy:'skip'}:{})})}))
 expect(vi.mocked(request).mock.calls.filter(([path,options])=>path==='/jobs'&&options?.method==='POST')).toHaveLength(0)
 expect(router.currentRoute.value.path).toBe(`/${kind}`);w.unmount()
})
it('loads an editor record directly even when it is not on the first resource page',async()=>{
 const id='00000000-0000-4000-8000-000000000001'
 vi.mocked(request).mockImplementation(async(path)=>{if(path==='/bootstrap')return {defaults:config,policy:{allow_raw:false,allow_credentials:true,allow_capture:true,allow_onvif:true}} as never;if(path==='/profiles/'+id)return {id,name:'Owned later-page record',config} as never;return {items:[],total:1000} as never})
 const router=createRouter({history:createMemoryHistory(),routes:[{path:'/:pathMatch(.*)*',component:{template:'<div />'}}]});await router.push('/profiles/'+id+'/edit');await router.isReady()
 const w=mount({components:{ConfigEditorPage,QLayout,QPageContainer},setup:()=>({id}),template:'<q-layout><q-page-container><ConfigEditorPage kind="profiles" :id="id"/></q-page-container></q-layout>'},{attachTo:document.body,global:{plugins:[router,[Quasar,{}]]}});await flushPromises()
 expect(request).toHaveBeenCalledWith('/profiles/'+id,expect.anything());expect(w.get('input[name="configuration_name"]').element).toHaveProperty('value','Owned later-page record');w.unmount()
})
