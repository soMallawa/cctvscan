import {it,expect} from 'vitest'
import {mount} from '@vue/test-utils'
import {Quasar} from 'quasar'
import ConfigFields from './ConfigFields.vue'
import type {ScanConfig} from '../types'
export const fixture:ScanConfig={targets:['192.0.2.1'],port_mode:'camera',ports:'',scan_mode:'connect',rate:1000,retry:3,wait:1,threads:25,adapter:'',adapter_ip:'',router_ip:'',router_mac:'',route_target:'',timeout_seconds:1800,host_timeout_seconds:30,host_workers:5,http_requests:5,http_per_host:2,http_rps:10,max_targets:1048576,proxy_version_id:'',credential_set_id:'',check_credentials:false,capture:false,onvif:false,debug:false}
it('exposes the scope, HTTP pacing and explicit sensitive-feature controls',()=>{
 const w=mount(ConfigFields,{props:{modelValue:fixture,policy:{allow_raw:false,allow_credentials:true,allow_capture:true,allow_onvif:true}},global:{plugins:[[Quasar,{}]]}})
 expect(w.find('[data-testid="targets"]').exists()).toBe(true)
 expect(w.find('[data-testid="http_rps"]').exists()).toBe(true)
 expect(w.text()).toContain('Credential checks');expect(w.text()).toContain('HTTP still images');expect(w.text()).toContain('ONVIF UDP discovery')
 expect(w.text()).toContain('Proxy applies to HTTP/HTTPS requests.');expect(w.text()).toContain('IP_FROM-IP_TO');w.unmount()
})
