import {it,expect} from 'vitest'
import {router} from './router'
it.each(['/results','/runners','/stats','/settings','/proxies','/credential-sets','/profiles','/profiles/new','/profiles/00000000-0000-4000-8000-000000000001/edit','/schedules','/schedules/new','/jobs/00000000-0000-4000-8000-000000000001'])('connects reachable console screen %s',path=>{
 expect(router.resolve(path).matched.at(-1)?.path).not.toBe('/:pathMatch(.*)*')
})
