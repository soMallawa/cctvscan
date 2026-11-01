import { test } from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
const manifest = new URL('../package.json', import.meta.url)
test('frontend uses the real Quasar framework and reproducible validation scripts', () => {
  assert.ok(existsSync(manifest), 'Quasar frontend manifest must exist')
  const pkg = JSON.parse(readFileSync(manifest, 'utf8'))
  assert.ok(pkg.dependencies.quasar, 'real Quasar dependency required')
  assert.ok(pkg.devDependencies['@quasar/vite-plugin'], 'official integration required')
  for (const name of ['build', 'typecheck', 'test', 'e2e']) assert.ok(pkg.scripts[name], `${name} gate required`)
  for (const version of Object.values({...pkg.dependencies, ...pkg.devDependencies})) assert.match(version, /^\d+\.\d+\.\d+$/, 'dependencies must be exactly pinned')
})
