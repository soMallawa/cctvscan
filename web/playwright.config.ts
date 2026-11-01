import {defineConfig} from '@playwright/test'
export default defineConfig({testDir:'./e2e',workers:1,timeout:60000,use:{baseURL:process.env.CCTVSCAN_E2E_ORIGIN||'http://127.0.0.1:14910',headless:true,screenshot:'only-on-failure',launchOptions:{executablePath:process.env.CCTVSCAN_CHROMIUM_PATH||undefined,args:['--no-sandbox','--disable-dev-shm-usage']}}})
