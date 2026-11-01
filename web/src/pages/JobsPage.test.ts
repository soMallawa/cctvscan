import {it,expect,vi,afterEach} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {Quasar,QLayout,QPageContainer} from 'quasar'
import {createRouter,createMemoryHistory} from 'vue-router'
import {request} from '../lib/api'
import JobsPage from './JobsPage.vue'
vi.mock('../lib/api',()=>({request:vi.fn()}))
afterEach(()=>vi.clearAllMocks())
it('shows a real empty queue without fabricated jobs or runner health and opens the composer',async()=>{
 vi.mocked(request).mockResolvedValue({items:[],total:0,counts:{queued:0,running:0}})
 const router=createRouter({history:createMemoryHistory(),routes:[{path:'/:pathMatch(.*)*',component:{template:'<div />'}}]});await router.push('/jobs');await router.isReady()
 const w=mount({components:{JobsPage,QLayout,QPageContainer},template:'<q-layout><q-page-container><JobsPage/></q-page-container></q-layout>'},{global:{plugins:[router,[Quasar,{}]]}});await flushPromises()
 expect(w.text()).toContain('No jobs yet');expect(w.text()).not.toContain('Lab baseline');expect(w.text()).not.toContain('Runner ready')
 await w.get('[data-testid="new-scan"]').trigger('click');await flushPromises();expect(router.currentRoute.value.path).toBe('/jobs/new');w.unmount()
})
it('does not misreport an unavailable queue as a verified empty queue',async()=>{
 vi.mocked(request).mockRejectedValue(new Error('Connection unavailable'))
 const router=createRouter({history:createMemoryHistory(),routes:[{path:'/:pathMatch(.*)*',component:{template:'<div />'}}]});await router.push('/jobs');await router.isReady()
 const w=mount({components:{JobsPage,QLayout,QPageContainer},template:'<q-layout><q-page-container><JobsPage/></q-page-container></q-layout>'},{global:{plugins:[router,[Quasar,{}]]}});await flushPromises()
 expect(w.text()).toContain('Not loaded');expect(w.text()).toContain('Queue unavailable');expect(w.text()).not.toContain('0 total');expect(w.text()).not.toContain('No jobs yet');w.unmount()
})
