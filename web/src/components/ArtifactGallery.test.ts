import {it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {Quasar} from 'quasar'
import ArtifactGallery from './ArtifactGallery.vue'
import {request} from '../lib/api'
vi.mock('../lib/api',()=>({request:vi.fn()}))
it('opens only protected, stored image content after an explicit click',async()=>{
 vi.mocked(request).mockResolvedValue({items:[{id:'00000000-0000-4000-8000-000000000001',media_type:'image/jpeg',size_bytes:512,sha256:'a'.repeat(64)}],total:1} as never)
 const w=mount(ArtifactGallery,{props:{jobId:'owned-job'},attachTo:document.body,global:{plugins:[[Quasar,{}]]}});await flushPromises();expect(request).toHaveBeenCalledWith('/jobs/owned-job/artifacts?limit=30&offset=0',expect.anything());expect(w.find('img').exists()).toBe(false)
 await w.get('[data-testid="view-stored-image"]').trigger('click');await flushPromises();const img=document.body.querySelector('img[data-testid="protected-image"]');expect(img?.getAttribute('src')).toBe('/api/artifacts/00000000-0000-4000-8000-000000000001');w.unmount()
})
it('does not label an unavailable artifact list as empty',async()=>{vi.mocked(request).mockRejectedValue(new Error('Storage unavailable'));const w=mount(ArtifactGallery,{props:{jobId:'owned-job'},global:{plugins:[[Quasar,{}]]}});await flushPromises();expect(w.text()).toContain('Storage unavailable');expect(w.text()).not.toContain('No stored images');w.unmount()})
