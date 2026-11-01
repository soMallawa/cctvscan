import {it,expect,vi,afterEach} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {Quasar,QLayout,QPageContainer,QInput,QBtn} from 'quasar'
import ResourcePage from './ResourcePage.vue'
import {request} from '../lib/api'
vi.mock('../lib/api',()=>({request:vi.fn()}))
afterEach(()=>vi.clearAllMocks())
it.each([['proxies','No proxy profiles'],['profiles','No scan profiles'],['schedules','No schedules'],['credential-sets','No credential sets']] as const)('loads actual %s metadata and shows its honest empty state',async(kind,heading)=>{
 vi.mocked(request).mockResolvedValue({items:[],total:0})
 const w=mount({components:{ResourcePage,QLayout,QPageContainer},setup:()=>({kind}),template:'<q-layout><q-page-container><ResourcePage :kind="kind"/></q-page-container></q-layout>'},{global:{plugins:[[Quasar,{}]]}});await flushPromises()
 expect(request).toHaveBeenCalledWith(`/${kind}?limit=10&offset=0&search=`,expect.anything());expect(w.text()).toContain(heading);expect(w.text()).not.toContain('Example proxy');w.unmount()
})
it('creates a proxy using a masked write-only URL and clears that URL after saving',async()=>{
 let created=false
 const proxy={id:'owned-proxy',version_id:'owned-version',name:'Owned proxy',scheme:'http',host:'127.0.0.1',port:19080,authentication_present:false,created_at:'2026-10-31T12:00:00Z',last_test_ok:null}
 vi.mocked(request).mockImplementation(async(path,options)=>{
  if(path==='/proxies'&&options?.method==='POST'){created=true;return proxy as never}
  return {items:created?[proxy]:[],total:created?1:0} as never
 })
 const w=mount({components:{ResourcePage,QLayout,QPageContainer},template:'<q-layout><q-page-container><ResourcePage kind="proxies"/></q-page-container></q-layout>'},{attachTo:document.body,global:{plugins:[[Quasar,{}]]}});await flushPromises()
 expect(w.find('[data-testid="add-resource"]').exists()).toBe(true)
 await w.get('[data-testid="add-resource"]').trigger('click');await flushPromises()
 const name=w.findAllComponents(QInput).find(c=>c.props('name')==='resource_name')!
 const url=w.findAllComponents(QInput).find(c=>c.props('name')==='proxy_url')!
 await name.get('input').setValue('Owned proxy');expect(url.get('input').attributes('type')).toBe('password');await url.get('input').setValue('http://127.0.0.1:19080')
 await w.findAllComponents(QBtn).find(c=>c.props('label')==='Save proxy')!.trigger('click');await flushPromises()
 expect(request).toHaveBeenCalledWith('/proxies',expect.objectContaining({method:'POST',body:{name:'Owned proxy',url:'http://127.0.0.1:19080'}}))
 expect(w.text()).toContain('Owned proxy');expect(w.text()).not.toContain('http://127.0.0.1:19080');expect((document.querySelector('input[name="proxy_url"]') as HTMLInputElement|null)?.value??'').toBe('');w.unmount()
})
it('imports credential content without returning or retaining it in the saved list',async()=>{
 let created=false;const content='fixture:'+crypto.randomUUID()
 const record={id:'owned-set',name:'Owned credentials',entry_count:1,created_at:'2026-10-31T12:00:00Z'}
 vi.mocked(request).mockImplementation(async(path,options)=>{if(path==='/credential-sets'&&options?.method==='POST'){created=true;return record as never}return {items:created?[record]:[],total:created?1:0} as never})
 const w=mount({components:{ResourcePage,QLayout,QPageContainer},template:'<q-layout><q-page-container><ResourcePage kind="credential-sets"/></q-page-container></q-layout>'},{attachTo:document.body,global:{plugins:[[Quasar,{}]]}});await flushPromises()
 expect(w.find('[data-testid="add-resource"]').exists()).toBe(true);await w.get('[data-testid="add-resource"]').trigger('click');await flushPromises()
 await w.findAllComponents(QInput).find(c=>c.props('name')==='resource_name')!.get('input').setValue('Owned credentials')
 await w.findAllComponents(QInput).find(c=>c.props('name')==='credential_content')!.get('textarea').setValue(content)
 await w.findAllComponents(QBtn).find(c=>c.props('label')==='Import encrypted set')!.trigger('click');await flushPromises()
 expect(request).toHaveBeenCalledWith('/credential-sets',expect.objectContaining({method:'POST',body:{name:'Owned credentials',content}}));expect(w.text()).toContain('1 encrypted entries');expect(w.text()).not.toContain(content);expect((document.querySelector('textarea[name="credential_content"]') as HTMLTextAreaElement|null)?.value??'').toBe('');w.unmount()
})
