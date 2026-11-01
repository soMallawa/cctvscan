import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { quasar, transformAssetUrls } from '@quasar/vite-plugin'
export default defineConfig({
 plugins: [vue({template:{transformAssetUrls}}), quasar()],
 server: {host:'127.0.0.1',port:14903,strictPort:true,proxy:{'/api':{target:process.env.CCTVSCAN_DEV_API || 'http://127.0.0.1:14904',changeOrigin:false}}},
 build:{target:'es2022',sourcemap:false},
 test:{environment:'happy-dom',include:['src/**/*.test.ts'],maxWorkers:1,clearMocks:true}
})
