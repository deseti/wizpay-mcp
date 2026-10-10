import { test } from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFileSync } from 'node:fs';
import { walletScript } from './wallet.mjs';
import { CDP } from './cdp.mjs';

function fixture() {
  const target = new EventTarget(); let provider, signed;
  class CustomEvent extends Event { constructor(type, init) { super(type); this.detail = init.detail; } }
  target.addEventListener('eip6963:announceProvider', e => { provider = e.detail.provider; });
  target.__wp3dSign = payload => { signed = JSON.parse(payload); target.__wp3dWalletResult(signed.id,'synthetic-signature'); };
  vm.runInNewContext(walletScript('0x'+'a'.repeat(40)), {window:target,location:{origin:'https://connect.wizpay.xyz'},CustomEvent,TextDecoder,TextEncoder,Uint8Array});
  target.dispatchEvent(new Event('eip6963:requestProvider'));
  return { target, provider, signed:() => signed };
}
test('discovery requests no account permissions', () => { const f=fixture(); assert.ok(f.provider); assert.deepEqual(Array.from(f.target.__wp3dWallet.state.calls),[]); });
test('exact UTF-8 personal_sign payload and address order', async () => { const f=fixture(), message='Exact\nmessage é🙂\n'; const hex='0x'+Buffer.from(message).toString('hex'); await f.provider.request({method:'personal_sign',params:[hex,'0x'+'a'.repeat(40)]}); assert.equal(f.signed().message,message); });
test('transaction methods and reversed signature parameters fail closed', async () => { const f=fixture(); for(const method of ['eth_sendTransaction','eth_sign','eth_signTypedData']) await assert.rejects(f.provider.request({method})); await assert.rejects(f.provider.request({method:'personal_sign',params:['0x'+'a'.repeat(40),'0x1234']})); });
test('permission and signature rejection controls', async () => { const f=fixture(); f.target.__wp3dWallet.state.rejectConnect=true; await assert.rejects(f.provider.request({method:'eth_requestAccounts'})); f.target.__wp3dWallet.state.rejectSign=true; await assert.rejects(f.provider.request({method:'personal_sign'})); });
test('listener cleanup and account change', () => { const f=fixture(); let n=0; const cb=()=>n++; f.provider.on('accountsChanged',cb); f.target.__wp3dWallet.emit('accountsChanged',[]); f.provider.removeListener('accountsChanged',cb); f.target.__wp3dWallet.emit('accountsChanged',[]); assert.equal(n,1); });
test('CDP rejects non-loopback and TLS-bypass endpoints', async () => { for(const url of ['ws://example.com/devtools/browser/id','ws://localhost/devtools/browser/id','http://127.0.0.1/devtools/browser/id']) await assert.rejects(CDP.connect(url)); });
test('browser override replaces publication and uses a distinct test database/image', () => {
  const config=readFileSync(new URL('./compose.override.yaml',import.meta.url),'utf8');
  assert.match(config,/ports: !override\s+- "127\.0\.0\.1:9443:8443"/);
  assert.match(config,/POSTGRES_DB: wizpay_mcp_browser_e2e/);
  assert.match(config,/wizpay-mcp-wp3d-browser-go:local/);
  assert.match(config,/APP_ENV: test/);
  assert.match(config,/WIZPAY_AUTONOMY_ENABLED: "false"/);
  assert.doesNotMatch(config,/privileged:|network_mode:|docker\.sock|0\.0\.0\.0/);
});

 test('browser argument gate rejects sandbox and TLS bypass', async()=>{
 const {validateBrowserArguments}=await import('./cdp.mjs');
 const good=['--enable-automation','--remote-debugging-address=127.0.0.1'];
 validateBrowserArguments(good);
 for(const flag of ['--no-sandbox','--ignore-certificate-errors','--ignore-certificate-errors-spki-list=bad','--disable-web-security']) assert.throws(()=>validateBrowserArguments([...good,flag]));
 assert.throws(()=>validateBrowserArguments([]));
 });
