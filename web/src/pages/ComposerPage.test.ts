import {it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {Quasar,QLayout,QPageContainer,QSelect} from 'quasar'
import ConfigFields from '../components/ConfigFields.vue'
import {createRouter,createMemoryHistory} from 'vue-router'
import {request} from '../lib/api'
import ComposerPage from './ComposerPage.vue'
vi.mock('../lib/api',()=>({request:vi.fn()}))
it('requires server preview and explicit scope authorization before queueing a scan',async()=>{
 const config={targets:['127.0.0.1'],port_mode:'custom',ports:'8080',scan_mode:'connect',rate:100,retry:0,wait:1,threads:1,adapter:'',adapter_ip:'',router_ip:'',router_mac:'',route_target:'',timeout_seconds:30,host_timeout_seconds:5,host_workers:1,http_requests:1,http_per_host:1,http_rps:1,max_targets:1,proxy_version_id:'',credential_set_id:'',check_credentials:false,capture:false,onvif:false,debug:false}
 vi.mocked(request).mockImplementation(async(path)=>{
  if(path==='/bootstrap')return {defaults:config,policy:{allow_raw:false,allow_credentials:true,allow_capture:true,allow_onvif:true},auth_mode:'local',version:'test'} as never
  if(path==='/jobs/preview')return {config,normalized_targets:config.targets,requested_address_upper_bound:1,unique_ports:1,effective_ports:'8080'} as never
  if(path==='/jobs')return {id:'owned-fixture',state:'queued'} as never
  return {items:[],total:0} as never
 })
 const router=createRouter({history:createMemoryHistory(),routes:[{path:'/:pathMatch(.*)*',component:{template:'<div />'}}]});await router.push('/jobs/new');await router.isReady()
 const w=mount({components:{ComposerPage,QLayout,QPageContainer},template:'<q-layout><q-page-container><ComposerPage/></q-page-container></q-layout>'},{attachTo:document.body,global:{plugins:[router,[Quasar,{}]]}});await flushPromises()
 expect(w.find('[data-testid="preview-job"]').exists()).toBe(true)
 await w.get('input[aria-label="Job name"]').setValue('Owned fixture')
 await w.get('[data-testid="preview-job"]').trigger('click');await flushPromises()
 expect(w.text()).toContain('Review execution');expect(w.get('[data-testid="queue-job"]').attributes('disabled')).toBeDefined()
 await w.get('[data-testid="authorize-scope"]').trigger('click');await flushPromises()
 expect(w.get('[data-testid="queue-job"]').attributes('disabled')).toBeUndefined()
 await w.get('[data-testid="queue-job"]').trigger('click');await flushPromises()
 expect(w.find('[role="alert"]').exists(),w.find('[role="alert"]').exists()?w.find('[role="alert"]').text():'').toBe(false)
 expect(request).toHaveBeenCalledWith('/jobs',expect.objectContaining({method:'POST',body:expect.objectContaining({name:'Owned fixture',config,idempotency_key:expect.any(String)})}))
 expect(router.currentRoute.value.path).toBe('/jobs/owned-fixture');w.unmount()
})
it('applies editable scenario templates without replacing targets or running a scan',async()=>{
 vi.clearAllMocks()
 const config={targets:['192.0.2.10'],port_mode:'custom',ports:'8080',scan_mode:'connect',rate:100,retry:0,wait:1,threads:1,adapter:'',adapter_ip:'',router_ip:'',router_mac:'',route_target:'',timeout_seconds:30,host_timeout_seconds:5,host_workers:1,http_requests:1,http_per_host:1,http_rps:1,max_targets:1,proxy_version_id:'',credential_set_id:'',check_credentials:false,capture:false,onvif:false,debug:false}
 vi.mocked(request).mockImplementation(async(path)=>{
  if(path==='/bootstrap')return {defaults:config,policy:{allow_credentials:true,allow_capture:true,allow_onvif:true}} as never
  if(path==='/credential-sets')return {items:[{id:'default-list',name:'CCTVScan defaults',entry_count:322}],total:1} as never
  return {items:[],total:0} as never
 })
 const router=createRouter({history:createMemoryHistory(),routes:[{path:'/:pathMatch(.*)*',component:{template:'<div />'}}]});await router.push('/jobs/new');await router.isReady()
 const w=mount({components:{ComposerPage,QLayout,QPageContainer},template:'<q-layout><q-page-container><ComposerPage/></q-page-container></q-layout>'},{attachTo:document.body,global:{plugins:[router,[Quasar,{}]]}});await flushPromises()
 const picker=w.findAllComponents(QSelect).find(c=>c.props('label')==='Scan template');expect(picker,'scenario picker').toBeTruthy()
 picker!.vm.$emit('update:modelValue','http-proxy');await flushPromises()
 const fields=w.findComponent(ConfigFields)
 expect(fields.props('modelValue')).toMatchObject({targets:['192.0.2.10'],port_mode:'custom',ports:'80,443',scan_mode:'connect',onvif:false,capture:false,check_credentials:true,credential_set_id:'default-list',rate:100})
 await w.get('input[aria-label="Job name"]').setValue('Template fixture')
 await w.get('[data-testid="preview-job"]').trigger('click');await flushPromises()
 expect(w.text()).toContain('Select a proxy profile for this template.')
 expect(request).not.toHaveBeenCalledWith('/jobs/preview',expect.anything())
 picker!.vm.$emit('update:modelValue','manual');await flushPromises();expect(fields.props('modelValue').ports).toBe('80,443')
 picker!.vm.$emit('update:modelValue','camera-connect');await flushPromises();expect(fields.props('modelValue')).toMatchObject({port_mode:'camera',scan_mode:'connect',check_credentials:false,onvif:false})
 picker!.vm.$emit('update:modelValue','http-direct');await flushPromises();expect(fields.props('modelValue')).toMatchObject({port_mode:'custom',ports:'80,443',proxy_version_id:'',onvif:false,check_credentials:true})
 expect(request).not.toHaveBeenCalledWith('/jobs',expect.anything());w.unmount()
})
