const fs = require('node:fs');
const path = require('node:path');
const {pathToFileURL} = require('node:url');
const {spawnSync} = require('node:child_process');
require('./prepare-webui-browser-test.cjs');
const chrome = process.env.CHROME_PATH || (process.platform === 'win32' ? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe' : process.platform === 'darwin' ? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : 'google-chrome');
const result = spawnSync(chrome, ['--headless', '--disable-gpu', '--disable-background-networking', '--no-first-run', '--no-default-browser-check', ...(process.env.CHROME_TEST_NO_SANDBOX === '1' ? ['--no-sandbox'] : []), '--user-data-dir=' + path.resolve('.build/chrome-test-profile-run'), '--virtual-time-budget=5000', '--dump-dom', pathToFileURL(path.resolve('.build/webui-browser-test.html')).href], {encoding:'utf8', windowsHide:true, timeout:30000, maxBuffer:4*1024*1024});
fs.writeFileSync('.build/webui-browser-dom.html', result.stdout || '');
if (!result.stdout?.includes('data-browser-test="passed"')) {
  console.error(result.error || 'Browser test failed, exit code ' + result.status);
  console.error((result.stderr || '').slice(0,2000));
  console.error(result.stdout?.match(/<pre id="browser-test-error">([\s\S]*?)<\/pre>/)?.[1] || 'No browser test result');
  process.exitCode = 1;
} else console.log('Chrome DOM integration: brand/subscription order, node frame preservation, connections, DNS and restart OK');
