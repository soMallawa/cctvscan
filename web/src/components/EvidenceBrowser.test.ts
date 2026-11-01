import {it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {Quasar} from 'quasar'
import EvidenceBrowser from './EvidenceBrowser.vue'
import {request} from '../lib/api'
vi.mock('../lib/api',()=>({request:vi.fn()}))
async function inspect(result:Record<string,unknown>){
 vi.mocked(request).mockResolvedValue({items:[{id:'owned-result',job_id:'owned-job',target:'127.0.0.1',brand:'Fixture',state:'inconclusive',result}],total:1})
 const w=mount(EvidenceBrowser,{global:{plugins:[[Quasar,{}]]}});await flushPromises()
 await w.get('[data-testid="inspect-result"]').trigger('click');await flushPromises();return w
}
it('shows stored evidence as evidence, without making the browser probe or play device URLs',async()=>{
 const w=await inspect({Ports:[554],CredentialState:'inconclusive',StreamResults:[{URL:'rtsp://127.0.0.1/fixture',State:'candidate'}]})
 expect(w.text()).toContain('127.0.0.1');expect(w.text()).toContain('inconclusive');expect(w.text()).toContain('CredentialState');expect(w.find('video').exists()).toBe(false);expect(w.find('iframe').exists()).toBe(false);expect(w.find('a[href^="rtsp:"]').exists()).toBe(false);expect(w.find('img').exists()).toBe(false);w.unmount()
})
const malicious='<html><img src="https://device.invalid/image"><script>alert(1)</script>café</html>'
const rootEvidence={Version:1,Method:'GET',Authenticated:false,CapturedAt:'2026-11-01T00:00:00Z',SHA256:'a'.repeat(64),BytesBase64:btoa(String.fromCharCode(...new TextEncoder().encode(malicious))),CapturedBytes:new TextEncoder().encode(malicious).length,Limit:8192,Truncated:true,WWWAuthenticate:'Basic realm="Fixture"'}
it('inspects stored root bytes as inert UTF-8 text and downloads only plaintext',async()=>{
 const fetchSpy=vi.spyOn(globalThis,'fetch');const create=vi.spyOn(URL,'createObjectURL').mockReturnValue('blob:fixture');const revoke=vi.spyOn(URL,'revokeObjectURL').mockImplementation(()=>{});const click=vi.spyOn(HTMLAnchorElement.prototype,'click').mockImplementation(()=>{})
 const w=await inspect({HTTPMeta:{Endpoints:[{URL:'http://127.0.0.1/',FinalURL:'http://127.0.0.1/login',StatusCode:401,Server:'Fixture server',ContentType:'text/html',State:'completed',RootEvidence:rootEvidence}]}})
 const endpoint=w.get('[data-testid="http-endpoint"]');expect(endpoint.attributes('open')).toBeUndefined();await endpoint.get('summary').trigger('click')
 expect(endpoint.text()).toContain('401');expect(endpoint.text()).toContain('Fixture server');expect(endpoint.text()).toContain('text/html');expect(endpoint.text()).toContain('http://127.0.0.1/login')
 expect(endpoint.text()).toContain('Unauthenticated GET');expect(endpoint.text()).toContain(rootEvidence.SHA256);expect(endpoint.text()).toContain('Truncated');expect(endpoint.text()).toContain('8192');expect(endpoint.text()).toContain('Basic realm="Fixture"');expect(endpoint.text()).toContain(rootEvidence.CapturedAt)
 expect(endpoint.get('pre').text()).toBe(malicious);expect(w.find('img, iframe, video, script').exists()).toBe(false);expect(fetchSpy).not.toHaveBeenCalled()
 expect(w.get('[data-testid="raw-evidence"]').attributes('open')).toBeUndefined()
 await endpoint.get('button').trigger('click');expect(create).toHaveBeenCalledOnce();const blob=create.mock.calls[0]![0] as Blob;expect(blob.type).toBe('text/plain;charset=utf-8');expect(await blob.text()).toBe(malicious);expect((click.mock.instances[0] as HTMLAnchorElement).download).toMatch(/\.txt$/);expect(revoke).toHaveBeenCalledWith('blob:fixture')
 w.unmount();fetchSpy.mockRestore();create.mockRestore();revoke.mockRestore();click.mockRestore()
})
it.each([
 [undefined,'not retained for this endpoint'],
 [{...rootEvidence,BytesBase64:''},'No body retained'],
 [{...rootEvidence,BytesBase64:'%%%invalid'},'Invalid base64'],
 [{...rootEvidence,BytesBase64:'a'.repeat(11001)},'exceeds the preview limit'],
 [{...rootEvidence,BytesBase64:btoa('a'.repeat(8193))},'exceeds the preview limit'],
])('handles unavailable or invalid root evidence safely: %j',async(evidence,message)=>{
 const w=await inspect({HTTPMeta:{Endpoints:[{URL:'http://127.0.0.1/',State:'failed',Error:'Fixture timeout',RootEvidence:evidence}]}})
 const endpoint=w.get('[data-testid="http-endpoint"]');expect(endpoint.text()).toContain(message);expect(endpoint.text()).toContain('Fixture timeout');expect(endpoint.find('pre').exists()).toBe(false);expect(endpoint.find('button').exists()).toBe(false);w.unmount()
})
it('includes individual detection reasons without treating possible as supported',async()=>{
 const w=await inspect({CameraDetection:{State:'possible'},CameraDetections:[{State:'supported',Reason:'Vendor API'},{State:'possible',Reason:'Generic HTTP'}]})
 const triage=w.get('[data-testid="camera-triage"]');expect(triage.text()).toContain('Supported · Vendor API');expect(triage.text()).toContain('Possible · Generic HTTP');w.unmount()
})
it.each([
 [{IsCamera:false},'Unknown (legacy)'],
 [{IsCamera:true},'Legacy positive — unverified'],
 [{State:'unknown',Reason:'No camera evidence',EvidenceVersion:1},'Unknown'],
 [{State:'supported',Reason:'Vendor signature',EvidenceVersion:1},'Supported'],
 [{State:'possible',Reason:'Generic login',EvidenceVersion:1},'Possible'],
])('labels detection without promoting legacy results: %j',async(detection,label)=>{
 const result={CameraDetection:detection};const before=JSON.stringify(result);const w=await inspect(result)
 expect(w.get('thead').text()).toContain('Detection')
 expect(w.get('[data-testid="detection-label"]').text()).toBe(label)
 expect(w.get('[data-testid="camera-triage"]').text()).toContain(label)
 if('Reason' in detection)expect(w.get('[data-testid="camera-triage"]').text()).toContain(detection.Reason)
 expect(JSON.stringify(result)).toBe(before)
 expect(w.get('select[aria-label="Rows per page"]').findAll('option').map(o=>o.text())).toEqual(['10','25','50'])
 w.unmount()
})
