import {createApp} from 'vue'
import {Quasar,Notify,Dialog} from 'quasar'
import iconSet from 'quasar/icon-set/svg-material-icons'
import '@fontsource-variable/public-sans'
import 'quasar/src/css/index.sass'
import './styles/console.css'
import App from './App.vue'
import {router} from './router'
createApp(App).use(router).use(Quasar,{iconSet,plugins:{Notify,Dialog},config:{brand:{primary:'#e8c83c',secondary:'#253540',accent:'#e8c83c',dark:'#172026',positive:'#246349',negative:'#a32d36',warning:'#7a5f13',info:'#37647d'}}}).mount('#app')
