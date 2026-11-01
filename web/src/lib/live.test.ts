import {it,expect,vi} from 'vitest'
import {flushPromises} from '@vue/test-utils'
import {subscribeEvents} from './live'
import {request} from './api'
vi.mock('./api',()=>({request:vi.fn()}))
it('shares one stream, preserves bigint IDs as strings and closes after the final subscriber',async()=>{
 class FakeSource {static all:FakeSource[]=[];closed=false;onopen:((e:Event)=>void)|null=null;onerror:((e:Event)=>void)|null=null;onmessage:((e:MessageEvent)=>void)|null=null;listeners=new Map<string,((e:MessageEvent)=>void)[]>();constructor(public url:string){FakeSource.all.push(this)}addEventListener(k:string,fn:(e:MessageEvent)=>void){this.listeners.set(k,[...(this.listeners.get(k)??[]),fn])}close(){this.closed=true}emit(k:string,data:unknown){for(const fn of this.listeners.get(k)??[])fn({data:JSON.stringify(data)} as MessageEvent)}}
 vi.stubGlobal('EventSource',FakeSource);vi.mocked(request).mockResolvedValue({event_cursor:'9007199254740992'})
 const fn=vi.fn(),offA=subscribeEvents(fn),offB=subscribeEvents(vi.fn());await flushPromises()
 expect(FakeSource.all).toHaveLength(1);const stream=FakeSource.all[0]!;expect(stream.url).toContain('after=9007199254740992')
 stream.emit('state',{seq:'9007199254740993',kind:'state',job_id:'owned-job',at:'2026-10-31T12:00:00Z',data:{state:'queued'}})
 expect(fn).toHaveBeenCalledWith(expect.objectContaining({seq:'9007199254740993'}));stream.emit('log',{seq:'9007199254740994',kind:'log',job_id:'owned-job',at:'2026-10-31T12:00:00Z',data:{message:'Stored event'}});expect(fn).toHaveBeenCalledTimes(2);offA();expect(stream.closed).toBe(false);offB();expect(stream.closed).toBe(true);vi.unstubAllGlobals()
})
