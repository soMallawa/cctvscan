import { describe,it,expect,vi,afterEach } from 'vitest'
import { request,ApiError } from './api'
afterEach(()=>vi.unstubAllGlobals())
describe('same-origin console API client',()=>{
 it('requests JSON without caching private results or accepting HTML as a successful result',async()=>{
  const fetcher=vi.fn().mockResolvedValue(new Response('<html>Access</html>',{status:200,headers:{'Content-Type':'text/html'}}));vi.stubGlobal('fetch',fetcher)
  await expect(request('/bootstrap')).rejects.toThrow(/Access|JSON|console/i)
  expect(fetcher.mock.calls[0][0]).toBe('/api/bootstrap');expect(fetcher.mock.calls[0][1].cache).toBe('no-store');expect(fetcher.mock.calls[0][1].credentials).toBe('same-origin')
 })
 it('sends mutations as JSON, passes abort signal and returns typed response',async()=>{
  const fetcher=vi.fn().mockResolvedValue(Response.json({id:'owned-fixture'}));vi.stubGlobal('fetch',fetcher);const c=new AbortController()
  expect(await request('/jobs',{method:'POST',body:{name:'fixture'},signal:c.signal})).toEqual({id:'owned-fixture'})
  expect(fetcher.mock.calls[0][1].body).toBe('{"name":"fixture"}');expect(fetcher.mock.calls[0][1].headers['Content-Type']).toBe('application/json');expect(fetcher.mock.calls[0][1].signal).toBe(c.signal)
 })
 it('keeps 401 distinct and does not surface raw upstream server errors',async()=>{
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue(Response.json({error:'postgres://private-secret@db'},{status:500})))
  try{await request('/jobs');throw Error('expected rejection')}catch(e){expect(e).toBeInstanceOf(ApiError);expect((e as Error).message).not.toContain('private-secret')}
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue(Response.json({error:'denied'},{status:401})))
  await expect(request('/jobs')).rejects.toMatchObject({status:401})
 })
 it('preserves safe field validation messages and handles 204 deletion',async()=>{
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue(Response.json({error:'targets: scope exceeds server limit',field:'targets'},{status:400})))
  await expect(request('/jobs')).rejects.toMatchObject({status:400,field:'targets',message:'targets: scope exceeds server limit'})
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response(null,{status:204})));expect(await request('/jobs/id',{method:'DELETE'})).toBeUndefined()
 })
 it('rejects absolute destinations so browser requests cannot bypass the console',async()=>{
  const fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);await expect(request('https://camera.invalid/image')).rejects.toThrow();expect(fetcher).not.toHaveBeenCalled()
 })
})
