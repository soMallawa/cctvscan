import {it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {Quasar,QLayout,QPageContainer} from 'quasar'
import SettingsPage from './SettingsPage.vue'
import {request} from '../lib/api'
vi.mock('../lib/api',()=>({request:vi.fn()}))
it('saves policy without resending server-only access metadata or implying Access protection in local mode',async()=>{
 const policy={max_targets:32,max_rate:100,max_threads:2,max_host_workers:2,max_http_requests:2,max_http_per_host:2,max_http_rps:10,max_retry:1,max_wait:5,max_timeout_seconds:60,max_host_timeout_seconds:10,allow_raw:false,allow_credentials:true,allow_capture:true,allow_onvif:true,allow_unpaced_http:true}
 const settings={policy,retention_days:30,max_concurrent_jobs:1,auth_mode:'local',public_origin:'http://127.0.0.1:14903',version:'fixture'}
 vi.mocked(request).mockResolvedValue(settings)
 const w=mount({components:{SettingsPage,QLayout,QPageContainer},template:'<q-layout><q-page-container><SettingsPage/></q-page-container></q-layout>'},{attachTo:document.body,global:{plugins:[[Quasar,{}]]}});await flushPromises()
 expect(w.get('[data-testid="access-mode"]').text()).toContain('Local development');expect(w.get('[data-testid="access-mode"]').text()).not.toContain('Cloudflare Access')
 await w.get('input[name="retention_days"]').setValue('60');await w.get('[data-testid="save-settings"]').trigger('click');await flushPromises()
 expect(request).toHaveBeenCalledWith('/settings',expect.objectContaining({method:'PUT',body:{policy,retention_days:60,max_concurrent_jobs:1}}));expect(w.text()).toContain('Settings saved');w.unmount()
})
