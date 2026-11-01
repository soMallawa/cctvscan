<script setup lang="ts">
import {ref} from 'vue'
import {matMenuBook,matViewList,matSearch,matDns,matAssessment,matSwapHoriz,matTune,matSchedule,matVpnKey,matSettings,matMenu} from '@quasar/extras/material-icons'
const drawer=ref(true)
const links=[['Jobs','/jobs',matViewList],['Results','/results',matSearch],['Camera catalogue','/catalogue',matMenuBook],['Runners','/runners',matDns],['Statistics','/stats',matAssessment],['Proxies','/proxies',matSwapHoriz],['Profiles','/profiles',matTune],['Schedules','/schedules',matSchedule],['Credential sets','/credential-sets',matVpnKey],['Settings','/settings',matSettings]]
</script>
<template>
 <q-layout view="hHh Lpr lFf" class="console-layout">
  <a href="#main-content" class="skip-link">Skip to content</a>
  <q-header class="mobile-header"><q-toolbar><q-btn flat round :icon="matMenu" aria-label="Open navigation" @click="drawer=!drawer"/><span class="brand">CCTVScan</span></q-toolbar></q-header>
  <q-drawer v-model="drawer" show-if-above :width="208" :breakpoint="900" class="navigation-rail">
   <div class="rail-inner"><router-link to="/jobs" class="brand brand-link"><span class="brand-mark" aria-hidden="true"/>CCTVScan</router-link>
   <nav aria-label="Main navigation"><router-link v-for="[label,path,icon] in links" :key="path" :to="path" class="nav-link"><q-icon :name="icon" size="19px"/><span>{{label}}</span></router-link></nav>
   <div class="rail-footer"><strong>Private workspace</strong><span>Authorized assessments only</span></div></div>
  </q-drawer>
  <q-page-container><div id="main-content" tabindex="-1"><router-view :key="$route.path"/></div></q-page-container>
 </q-layout>
</template>
