import { test } from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFileSync } from 'node:fs';
import { walletScript } from './wallet.mjs';
import { CDP } from './cdp.mjs';
import { resolverRules, optionsFor } from './transport.mjs';
import { environment, diagnosticContains } from './policy.mjs';
import { activationSQL } from './enrollment.mjs';
import { Faults } from './faults.mjs';
import { reliabilityStages } from './reliability.mjs';

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
 const good=['--enable-automation','--remote-debugging-address=127.0.0.1','--remote-debugging-port=9222','--no-proxy-server','--disable-extensions','--disable-background-networking','--disable-quic','--host-resolver-rules='+resolverRules,'--user-data-dir=/synthetic/fresh-profile'];
 validateBrowserArguments(good);
 for(const flag of ['--no-sandbox','--ignore-certificate-errors','--ignore-certificate-errors-spki-list=bad','--disable-web-security']) assert.throws(()=>validateBrowserArguments([...good,flag]));
 assert.throws(()=>validateBrowserArguments([]));
 for(const flag of ['--no-proxy-server','--host-resolver-rules='+resolverRules,'--user-data-dir=/synthetic/fresh-profile']) assert.throws(()=>validateBrowserArguments(good.filter(a=>a!==flag)));
 assert.throws(()=>validateBrowserArguments([...good,'--host-resolver-rules=MAP * example.com']));
 for(const flag of ['--remote-debugging-address=0.0.0.0','--remote-debugging-port=9223','--user-data-dir=','--proxy-server=public.example','--host-rules=MAP * example.com']) assert.throws(()=>validateBrowserArguments([...good,flag]));
 assert.throws(()=>validateBrowserArguments(good,'/different/profile'));
 });

test('local HTTPS preserves canonical Host/SNI without DNS or privileged port',()=>{
 const ca=Buffer.from('synthetic-public-ca'), opts=optionsFor('https://connect.wizpay.xyz/oauth/token',ca,'synthetic-form');
 assert.equal(opts.hostname,'127.0.0.1');assert.equal(opts.port,9443);assert.equal(opts.servername,'connect.wizpay.xyz');
 assert.equal(opts.headers.Host,'connect.wizpay.xyz');assert.equal(opts.agent,false);assert.equal(opts.rejectUnauthorized,true);
 for(const url of ['https://connect.wizpay.xyz:9443/oauth/token','https://neon.example/oauth/token','http://connect.wizpay.xyz/oauth/token','https://user:secret@connect.wizpay.xyz/oauth/token','https://connect.wizpay.xyz/oauth/token?code=x','https://connect.wizpay.xyz/browser/siwe/verify']) assert.throws(()=>optionsFor(url,ca));
});

test('diagnostic assertions tolerate case while preserving blocked-authority wording',()=>{
 for(const text of ['Restart authorization','then restart from your AI client','RESTART']) assert.equal(diagnosticContains(text,'restart'),true);
 assert.equal(diagnosticContains('Authenticated, consent still required','restart'),false);
});

test('runtime reservation and debugger gates reject unsafe configuration',()=>{
 const env={PHASE_B_RUN_ID:'13cc761-01',WP3D_BROWSER_OWNER_APPROVED:'yes',WP3D_BROWSER_CDP:'ws://127.0.0.1:9222/devtools/browser/synthetic-id'};
 const config=environment(env,'/synthetic/repo');assert.equal(config.runtime,'/synthetic/repo/tests/browser-e2e/runtime/13cc761-01');assert.equal(config.project,'wizpay-mcp-phaseb-13cc761-01');
 for(const change of [{PHASE_B_RUN_ID:'../../production'},{WP3D_BROWSER_OWNER_APPROVED:'no'},{WP3D_BROWSER_CDP:'ws://public.example:9222/devtools/browser/id'},{WP3D_BROWSER_CDP:'ws://127.0.0.1:9222/devtools/browser/id?secret=x'}]) assert.throws(()=>environment({...env,...change},'/synthetic/repo'));
});

function publicFixture() {
 const id='a'.repeat(64);
 return {fixture_id:id,tenant_id:'wp3d-browser-'+id,user_id:'browser-user-'+id,other_user_id:'other-browser-user-'+id,binding_id:'binding-'+id,other_binding_id:'other-binding-'+id,client_id:'browser-client-'+id,owner_role:'wp3d_owner_'+id.slice(0,24),address:'0x'+'1'.repeat(40),other_address:'0x'+'2'.repeat(40)};
}
test('owner activation uses existing proof-gated boundary and never inserts proof',()=>{
 const sql=activationSQL(publicFixture(),['b'.repeat(64),'c'.repeat(64)]);
 assert.match(sql,/current_database\(\) <> 'wizpay_mcp_browser_e2e'/);assert.match(sql,/SET SESSION AUTHORIZATION wp3d_owner_/);
 assert.equal((sql.match(/SELECT admin_activate_wallet/g)??[]).length,2);
 assert.doesNotMatch(sql,/INSERT INTO wallet_enrollment|UPDATE wallet_bindings|password|signature/);
 for(const proofs of [[],['b'.repeat(64),'b'.repeat(64)],["x' injected",'c'.repeat(64)]]) assert.throws(()=>activationSQL(publicFixture(),proofs));
 assert.throws(()=>activationSQL({...publicFixture(),owner_role:'production_owner'},['b'.repeat(64),'c'.repeat(64)]));
});

class FakeCDP {
 calls=[]; on(listener){this.listener=listener;return()=>{this.listener=undefined;};}
 async call(method,params){this.calls.push({method,params});if(method==='Fetch.getResponseBody')return {body:JSON.stringify({state:'AUTHENTICATED',csrf:'synthetic',transaction_id:'original'}),base64Encoded:false};return {};}
}
test('fault controller counts requests once and aborts reads without replaying verification',async()=>{
 const cdp=new FakeCDP(),fault=new Faults(cdp,'tab',({path,response,counts})=>!response&&path==='/browser/session'&&counts.session===1?'fail':undefined);
 await fault.start();
 await fault.handle({requestId:'v1',request:{url:'https://connect.wizpay.xyz/browser/siwe/verify'}});
 await fault.handle({requestId:'v2',request:{url:'https://connect.wizpay.xyz/browser/siwe/verify'},responseStatusCode:200});
 await fault.handle({requestId:'s1',request:{url:'https://connect.wizpay.xyz/browser/session'}});
 await fault.handle({requestId:'s2',request:{url:'https://connect.wizpay.xyz/browser/session'}});
 assert.deepEqual(fault.counts,{session:2,verify:1,consent:0,logout:0});
 assert.equal(cdp.calls.filter(c=>c.method==='Fetch.failRequest').length,1);
 assert.equal(cdp.calls.some(c=>c.method==='Fetch.fulfillRequest'),false);await fault.stop();
});
test('fault corruption is restricted to real successful session reads',async()=>{
 const cdp=new FakeCDP(),fault=new Faults(cdp,'tab',()=>v=>{v.transaction_id='mismatched';});
 await assert.rejects(fault.handle({requestId:'v',request:{url:'https://connect.wizpay.xyz/browser/siwe/verify'},responseStatusCode:200}));
 assert.equal(cdp.calls.length,0);
 await fault.handle({requestId:'s',request:{url:'https://connect.wizpay.xyz/browser/session'},responseStatusCode:200});
 const result=cdp.calls.find(c=>c.method==='Fetch.fulfillRequest');assert.equal(JSON.parse(Buffer.from(result.params.body,'base64').toString()).transaction_id,'mismatched');
});
test('held responses remain bounded and are released during scoped cleanup',async()=>{
 const cdp=new FakeCDP(),fault=new Faults(cdp,'tab',()=> 'hold');await fault.start();
 await fault.handle({requestId:'l',request:{url:'https://connect.wizpay.xyz/browser/logout'},responseStatusCode:200});
 const held=await fault.held('/browser/logout');await fault.release(held);assert.equal(fault.holds.length,0);await fault.stop();
 assert.equal(cdp.calls.filter(c=>c.method==='Fetch.continueResponse').length,1);
});
test('Phase B topology has distinct services, fixed local database and no shared mounts/networks',()=>{
 const config=readFileSync(new URL('./compose.phase-b.yaml',import.meta.url),'utf8');
 for(const name of ['phaseb-edge','phaseb-backend','phaseb-ui','phaseb-db']) assert.match(config,new RegExp('  '+name+':'));
 assert.match(config,/127\.0\.0\.1:9443:8443/);assert.match(config,/@phaseb-db:5432\/wizpay_mcp_browser_e2e/);
 assert.match(config,/tmpfs: \[\/var\/lib\/postgresql\/data\]/);assert.match(config,/internal: true/);
 assert.doesNotMatch(config,/external:|env_file:|docker\.sock|privileged:|network_mode:|0\.0\.0\.0|8443:8443/);
 assert.match(config,/WIZPAY_ARC_ENABLED: "false"/);assert.match(config,/WIZPAY_CIRCLE_ENABLED: "false"/);
 assert.match(config,/WIZPAY_AUTONOMY_ENABLED: "false"/);
});
test('reliability suite declares all requested failure boundaries without changing expiration',()=>{
 for(const stage of ['POST_VERIFY_RECOVERY','POST_VERIFY_READ_FAILURE','SESSION_CORRELATION_MISMATCH','SUPERSEDED_VERIFICATION','DISPOSED_AUTHENTICATION','DELAYED_LOGOUT','EXPIRED_AUTHORIZATION','EXPIRED_ACCESS_REAUTH']) assert.ok(reliabilityStages.includes(stage));
 const source=readFileSync(new URL('./reliability.mjs',import.meta.url),'utf8');
 assert.doesNotMatch(source,/UPDATE oauth_|setTimeout.*600.*\/|refresh_token.*grant_type|eth_sendTransaction/);
 assert.match(source,/first\.expires_in\*1000/);assert.match(source,/fault\.counts\.session,2/);assert.match(source,/fault\.counts\.verify,1/);
});
