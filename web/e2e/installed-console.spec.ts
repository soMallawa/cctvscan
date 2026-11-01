import {test,expect,type Page} from '@playwright/test'

// No request interception: these tests require a disposable installed stack and
// an explicitly supplied, owned fixture address reachable by its real worker.
const target=process.env.CCTVSCAN_E2E_TARGET
const port=process.env.CCTVSCAN_E2E_PORT||'18080'
const run='QA owned fixture '+Date.now()
async function draft(page:Page,name:string){
 await page.goto('/jobs/new')
 await page.getByRole('textbox',{name:'Job name',exact:true}).fill(name)
 await page.getByLabel('Target IP addresses, CIDRs or ranges',{exact:true}).fill(target!)
 await page.getByLabel('Port selection',{exact:true}).click()
 await page.getByRole('option',{name:'Custom ports and ranges'}).click()
 await page.getByLabel('Custom ports',{exact:true}).fill(port)
 const labels:Record<string,string>={max_targets:'Maximum target addresses',retry:'Discovery retries',wait:'Discovery wait (seconds)',threads:'Discovery threads',host_workers:'Concurrent host workers',timeout_seconds:'Whole-job timeout (seconds)',host_timeout_seconds:'Per-host timeout (seconds)',http_rps:'HTTP requests / second'}
 for(const [key,value] of Object.entries({max_targets:'1',retry:'1',wait:'1',threads:'1',host_workers:'1',timeout_seconds:'30',host_timeout_seconds:'5',http_rps:'0'}))await page.getByLabel(labels[key],{exact:true}).fill(value)
}
async function dispatch(page:Page){
 await page.getByTestId('preview-job').click()
 await expect(page.getByRole('heading',{name:'Review execution',exact:true})).toBeVisible()
 await page.getByTestId('authorize-scope').click()
 await page.getByTestId('queue-job').click()
 await expect(page).toHaveURL(/\/jobs\/[a-f0-9-]{36}$/)
}

test.beforeEach(async()=>{expect(target,'Set CCTVSCAN_E2E_TARGET to an explicitly owned private fixture').toBeTruthy();expect(target).toMatch(/^(127\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/)})

test('installed console executes and persists a real scanner job with negative control',async({page,request},info)=>{
 test.setTimeout(120000)
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await draft(page,run)
 await dispatch(page)
 const path=new URL(page.url()).pathname
 await expect.poll(async()=>{const r=await request.get('/api'+path);expect(r.ok()).toBeTruthy();return (await r.json()).state},{timeout:45000}).toBe('completed')
 const saved=await (await request.get('/api'+path)).json()
 expect(saved.result_count).toBe(1);expect(saved.camera_count).toBe(0);expect(saved.stream_count).toBe(0)
 await page.reload()
 await expect(page.getByRole('heading',{name:run,exact:true})).toBeVisible()
 await expect(page.locator('.record-status .status-badge')).toHaveText('completed',{timeout:10000})
 await expect(page.getByText('Live events connected',{exact:true})).toBeVisible()
 await expect(page.locator('.event-timeline li').first()).toBeVisible()
 await page.getByRole('tab',{name:'Evidence',exact:true}).click()
 await expect(page.getByRole('main')).toContainText(target!)
 const results=await (await request.get('/api'+path+'/results')).json();expect(results.total).toBe(1)
 const report=await request.get('/api'+path+'/export');expect(report.ok()).toBeTruthy();expect(await report.text()).toContain(target!)
 await page.getByRole('tab',{name:'Configuration snapshot'}).click()
 await expect(page.locator('.snapshot-panel pre')).toContainText('"capture": false')
 await page.screenshot({path:info.outputPath('job-desktop.png'),fullPage:true})
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:info.outputPath('job-mobile.png'),fullPage:true})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy()
 expect(errors).toEqual([])
 await page.getByTestId('retry-job').click()
 await expect(page.getByTestId('confirm-retry')).toBeDisabled()
 await page.getByTestId('authorize-retry').click()
 await page.getByTestId('confirm-retry').click()
 await expect.poll(()=>new URL(page.url()).pathname).not.toBe(path)
 const retried=await (await request.get('/api'+new URL(page.url()).pathname)).json()
 expect(retried.retry_of).toBe(saved.id)
})

test('delayed queued work can be canceled from its persistent record',async({page,request})=>{
 await draft(page,run+' delayed')
 await page.getByRole('checkbox',{name:'Delay this one job'}).click()
 const future=new Date(Date.now()+3600000);const local=new Date(future.getTime()-future.getTimezoneOffset()*60000).toISOString().slice(0,16)
 await page.getByLabel('Not before (your local time)',{exact:true}).fill(local)
 await dispatch(page)
 await page.getByTestId('cancel-job').click();await page.getByTestId('confirm-cancel').click()
 await expect.poll(async()=>{return (await (await request.get('/api'+new URL(page.url()).pathname)).json()).state}).toBe('canceled')
 await page.reload();await expect(page.locator('.record-status .status-badge')).toHaveText('canceled')
})

test('write-only proxy creation, replacement and removal through actual origin',async({page,request})=>{
 const name=run+' proxy';await page.goto('/proxies');await page.getByTestId('add-resource').click()
 await page.getByLabel('Profile name',{exact:true}).fill(name)
 await page.getByLabel('Full HTTP / HTTPS proxy URL',{exact:true}).fill('http://qa-user:qa-secret-never-return@127.0.0.1:18081')
 await page.getByRole('button',{name:'Save proxy',exact:true}).click()
 const row=page.getByRole('row').filter({hasText:name});await expect(row).toBeVisible();await expect(row).toContainText('Authentication configured')
 const response=await request.get('/api/proxies');expect(await response.text()).not.toContain('qa-secret-never-return')
 await row.getByRole('button',{name:'Replace',exact:true}).click();await expect(page.getByLabel('Full HTTP / HTTPS proxy URL',{exact:true})).toHaveValue('')
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 await row.getByRole('button',{name:'Delete '+name,exact:true}).click();await page.getByRole('button',{name:'Delete saved resource'}).click();await expect(row).toHaveCount(0)
})

test('all installed routes load at desktop and mobile widths',async({browser},info)=>{
 test.setTimeout(240000)
 const errors:string[]=[]
 for(const width of [1440,390]){
  for(const route of ['/jobs','/results','/runners','/stats','/proxies','/credential-sets','/profiles','/schedules','/settings']){
   const context=await browser.newContext({viewport:{width,height:900}})
   const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
   try{
    const apiPath=route==='/stats'?'/api/overview':'/api'+route
    const loaded=page.waitForResponse(r=>new URL(r.url()).pathname===apiPath&&r.request().method()==='GET')
    await page.goto(route,{waitUntil:'domcontentloaded'});expect((await loaded).ok()).toBeTruthy()
    await expect(page.locator('h1')).toBeVisible();await expect(page.locator('[role="alert"]')).toHaveCount(0)
    await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy()
    await page.screenshot({path:info.outputPath(route.slice(1)+'-'+width+'.png'),fullPage:true})
   }finally{await context.close()}
  }
 }
 expect(errors).toEqual([])
})
