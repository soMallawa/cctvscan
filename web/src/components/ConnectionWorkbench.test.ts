import {it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import ConnectionWorkbench from './ConnectionWorkbench.vue'
import {request} from '../lib/api'
vi.mock('../lib/api',()=>({request:vi.fn()}))
it('shows saved automatic verification without asking for credentials or making device requests',async()=>{
 vi.mocked(request).mockResolvedValue({assessment:{state:'brand_only',brand:'Fixture',reason:'Brand only',candidates:[]}})
 const w=mount(ConnectionWorkbench,{props:{resultId:'7',result:{AutomaticVerification:[{profile_id:'fixture-reference',attempted_pairs:1,credential_vendor:'Fixture',outcome:{state:'frame_verified',authentication:'accepted',media:'decoded_frame',reason:'Decoded fixture frame',width:16,height:12,route:'direct'}}]}},global:{stubs:{RouterLink:{template:'<a><slot/></a>'}}}});await flushPromises()
 expect(w.text()).toContain('Frame verified');expect(w.text()).toContain('Accepted');expect(w.text()).toContain('16 × 12');expect(w.find('input').exists()).toBe(false);expect(vi.mocked(request).mock.calls.every(([path,o])=>path==='/results/7/catalogue'&&!o?.method)).toBe(true);w.unmount()
})
