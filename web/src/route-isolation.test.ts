import {it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import {onUnmounted} from 'vue'
import {Quasar} from 'quasar'
import {createMemoryHistory,createRouter} from 'vue-router'
import App from './App.vue'
vi.mock('./lib/api',()=>({request:vi.fn(async()=>({status:'ok',auth_mode:'local',version:'fixture'}))}))
it('unmounts old editor state when changing record IDs',async()=>{let unmounted=0;const Page={setup(){onUnmounted(()=>unmounted++);return{}},template:'<div>Owned fixture editor</div>'};const router=createRouter({history:createMemoryHistory(),routes:[{path:'/profiles/:id/edit',component:Page}]});await router.push('/profiles/a/edit');await router.isReady();const w=mount(App,{global:{plugins:[router,[Quasar,{}]]}});await flushPromises();await router.push('/profiles/b/edit');await flushPromises();expect(unmounted).toBe(1);w.unmount()})
