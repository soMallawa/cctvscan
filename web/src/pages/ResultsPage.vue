<script setup lang="ts">
import {computed} from 'vue'
import {useRoute} from 'vue-router'
import EvidenceBrowser from '../components/EvidenceBrowser.vue'
const route=useRoute(),jobID=computed(()=>typeof route.query.job_id==='string'?route.query.job_id:'')
</script>
<template><q-page class="workspace"><header class="page-header"><div><p class="page-kicker">Evidence inventory</p><h1>Results</h1><p class="page-description">Browse scan results.</p></div></header><EvidenceBrowser :job-id="jobID"/></q-page></template>
