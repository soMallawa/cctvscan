import {it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {Quasar,QLayout,QPageContainer} from 'quasar'
import TelemetryPage from './TelemetryPage.vue'
import {request} from '../lib/api'
vi.mock('../lib/api',()=>({request:vi.fn()}))
it('does not invent connected runners before a real heartbeat',async()=>{
 vi.mocked(request).mockResolvedValue({items:[],total:0})
 const w=mount({components:{TelemetryPage,QLayout,QPageContainer},template:'<q-layout><q-page-container><TelemetryPage kind="runners"/></q-page-container></q-layout>'},{global:{plugins:[[Quasar,{}]]}});await flushPromises();expect(w.text()).toContain('No runners connected');expect(w.text()).not.toContain('Healthy');w.unmount()
})
it('keeps completed and inconclusive job counts visibly separate',async()=>{
 vi.mocked(request).mockResolvedValue({counts:{queued:0,running:0,completed:1,inconclusive:2,failed:0,canceled:0,timed_out:0,orphaned:0},result_count:0,camera_count:0,stream_count:0,series:[{day:'2026-10-31',jobs:3,completed:1,failed:0}],recent_jobs:[],runners:[]})
 const w=mount({components:{TelemetryPage,QLayout,QPageContainer},template:'<q-layout><q-page-container><TelemetryPage kind="stats"/></q-page-container></q-layout>'},{global:{plugins:[[Quasar,{}]]}});await flushPromises();expect(w.get('[data-state="completed"] strong').text()).toBe('1');expect(w.get('[data-state="inconclusive"] strong').text()).toBe('2');expect(w.text()).toContain('UTC');w.unmount()
})
it('samples real overview counts every five seconds without inventing chart history',async()=>{
 vi.useFakeTimers({toFake:['setInterval','clearInterval','Date']})
 const snapshot={counts:{running:0,queued:0,completed:0,inconclusive:0},result_count:10,camera_count:0,stream_count:0,series:[],recent_jobs:[],runners:[]}
 vi.mocked(request).mockResolvedValueOnce(snapshot).mockResolvedValue({...snapshot,result_count:14,counts:{...snapshot.counts,running:1}})
 const w=mount({components:{TelemetryPage,QLayout,QPageContainer},template:'<q-layout><q-page-container><TelemetryPage kind="stats"/></q-page-container></q-layout>'},{global:{plugins:[[Quasar,{}]]}})
 try{await flushPromises();const chart=w.find('[data-testid="live-results-chart"]');expect(chart.exists()).toBe(true);expect(chart.attributes('data-samples')).toBe('1');await vi.advanceTimersByTimeAsync(5000);await flushPromises();expect(chart.attributes('data-samples')).toBe('2');expect(w.text()).toContain('14');expect(w.text()).toContain('5 seconds')}finally{w.unmount();vi.useRealTimers()}
})
