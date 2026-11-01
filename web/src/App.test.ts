import {it,expect,vi,afterEach} from 'vitest'
import {mount} from '@vue/test-utils'
import {Quasar} from 'quasar'
import {createRouter,createMemoryHistory} from 'vue-router'
import App from './App.vue'
afterEach(()=>{document.body.innerHTML=''})
it('provides accessible job and settings navigation without invented fleet history',async()=>{
 const router=createRouter({history:createMemoryHistory(),routes:[{path:'/:pathMatch(.*)*',component:{template:'<div>Route fixture</div>'}}]});await router.push('/');await router.isReady()
 const w=mount(App,{attachTo:document.body,global:{plugins:[router,[Quasar,{}]]}})
 expect(w.find('nav[aria-label="Main navigation"]').exists()).toBe(true)
 expect(w.text()).toContain('Jobs');expect(w.text()).toContain('Settings');expect(w.text()).not.toContain('Lab baseline');w.unmount()
})
