import {it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {Quasar,QLayout,QPageContainer} from 'quasar'
import CataloguePage from './CataloguePage.vue'
import {request} from '../lib/api'
vi.mock('../lib/api',()=>({request:vi.fn()}))
it('searches the real API and opens reference specs without claiming an observed camera',async()=>{
 vi.mocked(request).mockImplementation(async(path)=>path==='/catalogue/meta'?{manifest:{models:2,brands:1,profiles:1,revision:'pinned-fixture',license:'CC0-1.0'},brands:[{key:'fixture',name:'Fixture',count:2}]}:path.startsWith('/catalogue/models?')?{items:[{id:'fixture-a',brand:'Fixture',model:'A',type:'bullet',protocols:['rtsp'],resolution:'1080p'}],total:1}:path==='/catalogue/models/fixture-a'?{id:'fixture-a',brand:'Fixture',model:'A',resolution:{megapixels:2},sources:['https://manufacturer.example/a']}: {items:[]})
 const w=mount({components:{CataloguePage,QLayout,QPageContainer},template:'<q-layout><q-page-container><CataloguePage/></q-page-container></q-layout>'},{global:{plugins:[[Quasar,{}]],stubs:{RouterLink:{template:'<a><slot/></a>'}}}});await flushPromises()
 await w.get('[data-testid="camera-model"]').trigger('click');await flushPromises()
 expect(w.text()).toContain('Reference specifications');expect(w.text()).toContain('not observed device properties');expect(w.find('a[href="https://manufacturer.example/a"]').exists()).toBe(true)
 const input=w.get('input[aria-label="Search camera catalogue"]');await input.setValue('A');await input.trigger('keyup.enter');await flushPromises();expect(vi.mocked(request).mock.calls.some(([p])=>p.includes('search=A'))).toBe(true);w.unmount()
})
