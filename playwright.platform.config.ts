import { defineConfig, devices } from '@playwright/test'

const apiPort = process.env.COOKIES_E2E_API_PORT ?? '18080'
const webPort = process.env.COOKIES_E2E_WEB_PORT ?? '4174'
const apiBaseURL = `http://127.0.0.1:${apiPort}`
const webBaseURL = `http://127.0.0.1:${webPort}`
// Run through the repository's root compose.yaml so local acceptance reuses the
// canonical `cookies` project instead of creating a second MySQL container on
// the same host port.
const mysqlBootstrap = `docker compose up -d --wait mysql && docker compose exec -T mysql mysql -uroot -proot_local_development_only -e 'CREATE DATABASE IF NOT EXISTS cookies_e2e CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci'`
const mysqlBootstrapWindows = `docker compose up -d --wait mysql && docker compose exec -T mysql mysql -uroot -proot_local_development_only -e "CREATE DATABASE IF NOT EXISTS cookies_e2e CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"`
const mysqlCommand = process.env.COOKIES_E2E_SKIP_MYSQL_BOOTSTRAP === 'true'
  ? 'node -e ""'
  : process.platform === 'win32'
    ? mysqlBootstrapWindows
    : mysqlBootstrap
const editorFixtureBootstrap = process.platform === 'win32'
  ? 'docker build --target verifier -f deployments/render-worker/Dockerfile -t cookies-render-verifier:c7 . && docker run --rm --volume "%CD%:/workspace" --workdir /workspace cookies-render-verifier:c7 sh scripts/generate-video-editor-c0-fixtures.sh .tmp/video-editor-e2e && node scripts/prepare-video-editor-e2e-blobs.mjs'
  : 'docker build --target verifier -f deployments/render-worker/Dockerfile -t cookies-render-verifier:c7 . && docker run --rm --volume "$(pwd):/workspace" --workdir /workspace cookies-render-verifier:c7 sh scripts/generate-video-editor-c0-fixtures.sh .tmp/video-editor-e2e && node scripts/prepare-video-editor-e2e-blobs.mjs'
const localChromiumExecutable = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
const reuseE2EServers = process.env.COOKIES_E2E_REUSE_SERVERS === 'true'
const apiExecutable = process.platform === 'win32'
  ? '.cache\\runtime\\cookies-api-e2e.exe'
  : '.cache/runtime/cookies-api-e2e'
const runApiExecutable = process.platform === 'win32' ? `"${apiExecutable}"` : `./${apiExecutable}`

const localGoEnv = {
  COOKIES_ENV: 'local',
  COOKIES_PASSWORD_AUTH_ENABLED: 'false',
  COOKIES_HTTP_ADDR: `:${apiPort}`,
  COOKIES_MYSQL_DSN: 'root:root_local_development_only@tcp(127.0.0.1:3307)/cookies_e2e?parseTime=true&multiStatements=true',
  COOKIES_LOCAL_ORGANIZATION_ID: 'org_local',
  COOKIES_LOCAL_PRINCIPAL_KIND: 'user',
  COOKIES_LOCAL_PRINCIPAL_ID: 'user_local',
  COOKIES_LOCAL_PROJECT_ID: 'project_local',
  COOKIES_LOCAL_SCOPES: [
    'project.read',
    'project.write',
    'assets.read',
    'assets.write',
    'delivery.read',
    'delivery.write',
    'delivery.approve',
    'delivery.execute',
    'provider.job.create',
    'provider.text.generate',
    'provider.vision.understand',
    'strategy.read',
    'strategy.write',
    'strategy.confirm',
    'strategy.review',
    'strategy.approve',
    'strategy.package.read',
    'creative.read',
    'creative.write',
  ].join(','),
  COOKIES_STRATEGY_ENABLED: 'true',
  COOKIES_STRATEGY_V2_ENABLED: 'true',
  COOKIES_STRATEGY_REAL_PROVIDER_ENABLED: 'false',
  COOKIES_STRATEGY_CRITIC_ENABLED: 'false',
  COOKIES_STRATEGY_PACKAGE_TO_CREATIVE_ENABLED: 'true',
  COOKIES_STRATEGY_CREATIVE_TASK_PLANNING_ENABLED: 'true',
  COOKIES_CREATIVE_DIRECTION_PLANNING_ENABLED: 'false',
  COOKIES_PROVIDER_TEXT_ADAPTER: 'fake',
  COOKIES_BLOB_PROVIDER: 'filesystem',
  COOKIES_FILESYSTEM_BLOB_ROOT: '.data/e2e-platform-blobs',
  COOKIES_SCANNER_MODE: 'noop',
}

export default defineConfig({
  testDir: './e2e',
  testMatch: /(platform-go-demo|strategy-brand-video-foundation|delivery-plan-preflight|delivery-approval-content-hash|delivery-execution-scenarios|delivery-monitoring-alerts|delivery-three-tier|delivery-mock-tour|video-editor-phase1)\.spec\.ts/,
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: webBaseURL,
    trace: 'retain-on-failure',
  },
  webServer: [
    {
      command: `${mysqlCommand} && ${editorFixtureBootstrap} && go run ./cmd/cookies-migrate && go run ./cmd/cookies-seed && node -e "require('fs').mkdirSync('.cache/runtime',{recursive:true})" && go build -o "${apiExecutable}" ./cmd/cookies-api && ${runApiExecutable}`,
      url: `${apiBaseURL}/healthz`,
      env: {
        ...process.env,
        ...localGoEnv,
      },
      reuseExistingServer: reuseE2EServers,
      timeout: 300_000,
    },
    {
      command: `node node_modules/vite/bin/vite.js --host 127.0.0.1 --port ${webPort}`,
      url: webBaseURL,
      env: {
        ...process.env,
        VITE_PLATFORM_PROXY_TARGET: apiBaseURL,
      },
      reuseExistingServer: reuseE2EServers,
      timeout: 60_000,
    },
  ],
  projects: [{
    name: 'chromium',
    use: {
      ...devices['Desktop Chrome'],
      launchOptions: localChromiumExecutable ? { executablePath: localChromiumExecutable } : undefined,
    },
  }],
})
