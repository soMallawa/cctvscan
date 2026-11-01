import {createRouter,createWebHistory} from 'vue-router'
import JobsPage from './pages/JobsPage.vue'
import ComposerPage from './pages/ComposerPage.vue'
import NotFoundPage from './pages/NotFoundPage.vue'
export const router=createRouter({history:createWebHistory(),routes:[
 {path:'/',redirect:'/jobs'},
 {path:'/jobs',component:JobsPage},
 {path:'/jobs/new',component:ComposerPage},
 {path:'/jobs/:id',component:()=>import('./pages/JobDetailPage.vue'),props:true},
 {path:'/catalogue',component:()=>import('./pages/CataloguePage.vue'),props:(r)=>({initialSearch:String(r.query.search??'')})},
 {path:'/results',component:()=>import('./pages/ResultsPage.vue')},
 {path:'/runners',component:()=>import('./pages/TelemetryPage.vue'),props:{kind:'runners'}},
 {path:'/stats',component:()=>import('./pages/TelemetryPage.vue'),props:{kind:'stats'}},
 {path:'/settings',component:()=>import('./pages/SettingsPage.vue')},
 ...(['proxies','credential-sets','profiles','schedules'] as const).map(kind=>({path:'/'+kind,component:()=>import('./pages/ResourcePage.vue'),props:{kind}})),
 ...(['profiles','schedules'] as const).flatMap(kind=>[
  {path:'/'+kind+'/new',component:()=>import('./pages/ConfigEditorPage.vue'),props:{kind}},
  {path:'/'+kind+'/:id/edit',component:()=>import('./pages/ConfigEditorPage.vue'),props:(route:import('vue-router').RouteLocationNormalized)=>({kind,id:String(route.params.id)})}
 ]),
 {path:'/:pathMatch(.*)*',component:NotFoundPage}
],scrollBehavior:()=>({top:0})})
