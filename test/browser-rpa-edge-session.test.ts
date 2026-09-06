import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import test from 'node:test'
import { currentUserProfilePath, edgeArguments, isLikelyOceanEngineLogin, parseDevToolsActivePort, sanitizePageURL, sessionPaths } from '../scripts/browser-rpa-edge-session.js'
import { inspectOceanEnginePages } from '../scripts/browser-rpa-session-probe.js'

test('session files stay under LOCALAPPDATA and use the managed Default profile', () => {
  const localAppData = resolve('C:/Users/test/AppData/Local')
  const paths = sessionPaths(localAppData)
  const expectedRoot = join(localAppData, 'cookies', 'browser-rpa')
  assert.equal(paths.root, expectedRoot)
  assert.equal(paths.profile, join(expectedRoot, 'edge-user-data', 'Default'))
  for (const path of Object.values(paths)) assert.ok(path.startsWith(expectedRoot))
})

test('Edge arguments keep the browser visible and bind CDP to loopback', () => {
  const paths = sessionPaths(resolve('C:/Users/test/AppData/Local'))
  const args = edgeArguments(paths, 19222)
  assert.ok(args.includes('--profile-directory=Default'))
  assert.ok(args.includes('--remote-debugging-address=127.0.0.1'))
  assert.ok(args.includes('--remote-debugging-port=19222'))
  assert.ok(args.includes(`--user-data-dir=${paths.userData}`))
  assert.ok(args.includes('https://ad.oceanengine.com/'))
  assert.ok(!args.some(value => /headless/i.test(value)))
})

test('current profile mode points to the existing Edge Default Profile without copying it', () => {
  const localAppData = resolve('C:/Users/test/AppData/Local')
  assert.equal(currentUserProfilePath(localAppData), join(localAppData, 'Microsoft', 'Edge', 'User Data', 'Default'))
  assert.notEqual(currentUserProfilePath(localAppData), sessionPaths(localAppData).profile)
})

test('DevToolsActivePort resolves the direct Edge WebSocket', () => {
  assert.deepEqual(parseDevToolsActivePort('9222\n/devtools/browser/31c9459a-f827-47e9-94aa-b625164dfaae\n', 9222), {
    port: 9222,
    websocket_endpoint: 'ws://127.0.0.1:9222/devtools/browser/31c9459a-f827-47e9-94aa-b625164dfaae',
  })
  assert.throws(() => parseDevToolsActivePort('9222\n/not-devtools\n', 9222), /browser path/)
  assert.throws(() => parseDevToolsActivePort('9223\n/devtools/browser/31c9459a-f827-47e9-94aa-b625164dfaae\n', 9222), /does not match/)
})

test('page diagnostics remove query strings and fragments', () => {
  assert.deepEqual(sanitizePageURL('https://ad.oceanengine.com/path/item?token=secret#fragment'), {
    protocol: 'https:',
    host: 'ad.oceanengine.com',
    pathname: '/path/item',
  })
})

test('login check uses only the safe page location', () => {
  assert.equal(isLikelyOceanEngineLogin({ protocol: 'https:', host: 'ad.oceanengine.com', pathname: '/campaign' }), true)
  assert.equal(isLikelyOceanEngineLogin({ protocol: 'https:', host: 'ad.oceanengine.com', pathname: '/login' }), false)
  assert.equal(isLikelyOceanEngineLogin({ protocol: 'https:', host: 'open.oceanengine.com', pathname: '/' }), false)
})

test('session probe waits for a matching OceanEngine account URL', () => {
  assert.equal(inspectOceanEnginePages(['edge://newtab/'], '1855554434276391').reason, 'oceanengine_page_missing')
  assert.equal(inspectOceanEnginePages(['https://ad.oceanengine.com/login?aadvid=1855554434276391'], '1855554434276391').reason, 'login_required')
  assert.equal(inspectOceanEnginePages(['https://ad.oceanengine.com/campaign?aadvid=2'], '1855554434276391').reason, 'account_mismatch')
  assert.equal(inspectOceanEnginePages(['https://ad.oceanengine.com/campaign?aadvid=1855554434276391'], '1855554434276391').status, 'ready')
})

test('session check has no credential, storage, network, or page-write operation', () => {
  const source = [
    readFileSync(resolve(import.meta.dirname, '../scripts/browser-rpa-edge-session.ts'), 'utf8'),
    readFileSync(resolve(import.meta.dirname, '../scripts/browser-rpa-edge-attach-once.mjs'), 'utf8'),
    readFileSync(resolve(import.meta.dirname, '../scripts/browser-rpa-session-probe.ts'), 'utf8'),
  ].join('\n')
  for (const forbidden of [
    /\.cookies\s*\(/,
    /localStorage/,
    /sessionStorage/,
    /storageState\s*\(/,
    /\.getAllCookies\s*\(/,
    /\.setCookie\s*\(/,
    /\.setExtraHTTPHeaders\s*\(/,
    /\.route\s*\(/,
    /page\.goto\s*\(/,
    /\.click\s*\(/,
    /\.fill\s*\(/,
    /\.selectOption\s*\(/,
    /\.setInputFiles\s*\(/,
    /\.download\s*\(/,
  ]) assert.doesNotMatch(source, forbidden)
})

test('session probe falls back to the browser-level read-only target inventory', () => {
  const source = readFileSync(resolve(import.meta.dirname, '../scripts/browser-rpa-session-probe.ts'), 'utf8')
  assert.match(source, /Target\.getTargets/)
  assert.match(source, /readCDPPageURLs\(endpoint\)/)
  assert.doesNotMatch(source, /Target\.attachToTarget|Page\.navigate|Runtime\.evaluate/)
})

test('attachment disconnects CDP without closing the managed Edge browser', () => {
  const helperSource = readFileSync(resolve(import.meta.dirname, '../scripts/browser-rpa-edge-attach-once.mjs'), 'utf8')
  const sessionSource = readFileSync(resolve(import.meta.dirname, '../scripts/browser-rpa-edge-session.ts'), 'utf8')
  assert.doesNotMatch(helperSource, /browser\.close\s*\(/)
  assert.doesNotMatch(helperSource, /Browser\.close/)
  assert.match(helperSource, /chromium\.connectOverCDP\s*\(/)
  assert.match(helperSource, /process\.exit\s*\(0\)/)
  assert.doesNotMatch(helperSource, /contexts\.length\s*!==\s*1/)
  assert.match(helperSource, /browserContextId\s*\?\?\s*'default'/)
  assert.match(helperSource, /Date\.now\(\)\s*\+\s*15000/)
  assert.match(sessionSource, /timeout:\s*120000/)
})
