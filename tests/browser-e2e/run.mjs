// Owner-run only. Requires trusted local HTTPS, current isolated stack and Chrome.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { request as httpsRequest } from 'node:https';
import { randomBytes, createHash } from 'node:crypto';
import { CDP, validateBrowserArguments } from './cdp.mjs';
import { walletScript } from './wallet.mjs';

// Stage/checkpoint pairs and helper operation numbers are stable source locations.
// Do not renumber existing identifiers when adding future checkpoints.
// Diagnostic fields are static source identifiers. Never inspect or print error
// messages, stacks, protocol payloads, browser exception details, or URL values.
const checkpoints = new Map([
  ['INVALID_OAUTH_REDIRECT', new Set(['01', '02', '03'])],
  ['AUTHORIZATION_DENIAL', new Set(['01', '02', '03', '04'])],
  ['REJECTED_WALLET_SIGNING', new Set(['01', '02', '03', '04', '05'])],
  ['WRONG_NETWORK', new Set(['01', '02', '03', '04', '05', '06', '07', '08'])],
  ['WALLET_EVENT_INVALIDATION', new Set(['01', '02', '03', '04', '05', '06', '07', '08'])],
  ['CHALLENGE_EXPIRY', new Set(['01', '02', '03', '04', '05', '06', '07'])],
  ['MULTI_TAB_SESSION_ISOLATION', new Set(['01', '02', '03', '04', '05', '06', '07', '08', '09', '10', '11'])],
  ['INTERRUPTED_SIWE_TRANSPORT', new Set(['01', '02', '03', '04', '05', '06', '07', '08', '09'])],
  ['FIXTURE_INITIALIZATION', new Set(['01', '02', '03', '04', '05', '06', '07'])],
  ['CDP_CONNECTION', new Set(['01'])],
  ['CHROME_POLICY_VALIDATION', new Set(['01'])],
  ['BROWSER_CONTEXT_INITIALIZATION', new Set(['01', '02'])],
  ['OAUTH_AUTHORIZATION_ONBOARDING', new Set(['01', '02', '03', '04', '05', '06'])],
  ['SECURE_COOKIE_CSRF_VALIDATION', new Set(['01', '02', '03', '04', '05', '06', '07', '08', '09', '10'])],
  ['WALLET_CONNECTION_SIWE_AUTHENTICATION', new Set(['01', '02', '03'])],
  ['SIWE_REPLAY_REJECTION', new Set(['01', '02'])],
  ['SESSION_ROTATION_STALE_COOKIE_REJECTION', new Set(['01', '02', '03', '04', '05', '06'])],
  ['CONSENT_OAUTH_CALLBACK', new Set(['01', '02', '03', '04', '05', '06', '07', '08'])],
  ['PKCE_TOKEN_EXCHANGE', new Set(['01', '02', '03'])],
  ['MCP_ORIGIN_COOKIE_ISOLATION', new Set(['01', '02', '03', '04'])],
  ['MCP_INITIALIZATION', new Set(['01'])],
  ['MCP_TOOL_DISCOVERY', new Set(['01', '02'])],
  ['EXCLUDED_TOOL_REJECTION', new Set(['01', '02'])],
  ['MISSING_BEARER_REJECTION', new Set(['01', '02', '03'])],
  ['INVALID_BEARER_REJECTION', new Set(['01', '02', '03'])],
  ['LOGOUT_SESSION_INVALIDATION', new Set(['01', '02', '03', '04', '05'])],
]);
const operationIDs = new Set([
  'NONE', 'EVALUATE_01', 'EVALUATE_02', 'EVALUATE_03',
  'WAIT_01', 'WAIT_02', 'WAIT_03',
  'CLICK_01',
  'TAB_01', 'TAB_02', 'TAB_03', 'TAB_04', 'TAB_05', 'TAB_06', 'TAB_07', 'TAB_08', 'TAB_09',
  'CONNECT_01', 'CONNECT_02', 'CONNECT_03', 'CONNECT_04', 'CONNECT_05',
  'AUTHENTICATE_01', 'AUTHENTICATE_02', 'AUTHENTICATE_03', 'AUTHENTICATE_04', 'AUTHENTICATE_05', 'AUTHENTICATE_06', 'AUTHENTICATE_07', 'AUTHENTICATE_08', 'AUTHENTICATE_09', 'AUTHENTICATE_10', 'AUTHENTICATE_11', 'AUTHENTICATE_12', 'AUTHENTICATE_13',
  'PENDINGTAB_01', 'PENDINGTAB_02', 'PENDINGTAB_03',
]);
// CDP errors have no safe typed timeout/network discriminator. Keep them CDP;
// browser exceptionDetails stay UNKNOWN. Never classify from error text.
const categories = new Set(['ASSERTION', 'TIMEOUT', 'CDP', 'FIXTURE', 'UNKNOWN']);
const eventIDs = new Set(['NONE', 'ACCOUNTS_CHANGED', 'CHAIN_CHANGED', 'DISCONNECT']);
const excludedToolIDs = new Map([
  ['wizpay.create_intent', 'CREATE_INTENT'],
  ['wizpay.request_approval', 'REQUEST_APPROVAL'],
  ['wizpay.evaluate_policy', 'EVALUATE_POLICY'],
  ['wizpay.prepare_execution', 'PREPARE_EXECUTION'],
  ['wizpay.send.preview', 'SEND_PREVIEW'],
  ['wizpay.send.create_intent', 'SEND_CREATE_INTENT'],
  ['wizpay.send.execute', 'SEND_EXECUTE'],
  ['wizpay.send.status', 'SEND_STATUS'],
  ['wizpay.payroll.preview', 'PAYROLL_PREVIEW'],
  ['wizpay.payroll.create_intent', 'PAYROLL_CREATE_INTENT'],
  ['wizpay.payroll.execute', 'PAYROLL_EXECUTE'],
  ['wizpay.payroll.status', 'PAYROLL_STATUS'],
  ['wizpay.swap.preview', 'SWAP_PREVIEW'],
  ['wizpay.swap.create_intent', 'SWAP_CREATE_INTENT'],
  ['wizpay.swap.execute', 'SWAP_EXECUTE'],
  ['wizpay.swap.status', 'SWAP_STATUS'],
  ['wizpay.autonomy.list_schedules', 'AUTONOMY_LIST_SCHEDULES'],
  ['wizpay.autonomy.get_schedule', 'AUTONOMY_GET_SCHEDULE'],
  ['wizpay.autonomy.simulate', 'AUTONOMY_SIMULATE'],
  ['wizpay.autonomy.create_schedule', 'AUTONOMY_CREATE_SCHEDULE'],
  ['wizpay.autonomy.control_schedule', 'AUTONOMY_CONTROL_SCHEDULE'],
  ['wizpay.autonomy.emergency_stop', 'AUTONOMY_EMERGENCY_STOP'],
]);
const toolIDs = new Set(['NONE', ...excludedToolIDs.values()]);
let excludedTool = 'NONE';
const failures = new WeakMap(), operations = new WeakMap();
let currentStage = 'FIXTURE_INITIALIZATION', currentCheckpoint = '01', walletEvent = 'NONE';
function checkpoint(stage, point) {
  if (!checkpoints.get(stage)?.has(point)) throw failure('UNKNOWN');
  currentStage = stage; currentCheckpoint = point;
}
function failure(category) {
  const error = new Error('Browser diagnostic failure');
  failures.set(error, categories.has(category) ? category : 'UNKNOWN');
  return error;
}
async function categorized(category, action) {
  try { return await action(); } catch { throw failure(category); }
}
async function cdpCall(...args) {
  return categorized('CDP', () => cdp.call(...args));
}
function reportFailure(error) {
  const category = error instanceof assert.AssertionError ? 'ASSERTION' : failures.get(error);
  const operation = (operations.get(error) ?? []).filter(id => operationIDs.has(id)).join('.') || 'NONE';
  const stage = checkpoints.get(currentStage)?.has(currentCheckpoint) ? currentStage : 'UNKNOWN';
  console.error('FAIL: browser validation did not complete.' +
    ` stage=${stage} checkpoint=${stage === 'UNKNOWN' ? '00' : currentCheckpoint}` +
    ` operation=${operation}` +
    ` event=${eventIDs.has(walletEvent) ? walletEvent : 'NONE'}` +
    ` tool=${toolIDs.has(excludedTool) ? excludedTool : 'NONE'}` +
    ` category=${categories.has(category) ? category : 'UNKNOWN'}`);
}

const origin = 'https://connect.wizpay.xyz', resource = 'https://mcp.wizpay.xyz/mcp';
if (process.env.WP3D_BROWSER_OWNER_APPROVED !== 'yes') throw new Error('Owner approval for isolated browser execution required');
const signer = spawn('./deploy/local/runtime/browser-e2e-signer', [], {stdio:['pipe','pipe','ignore']});
const lines = createInterface({input:signer.stdout});
const waiting = new Map(), challenges = new Map(); let signingID=0, cdp, context;
const first = new Promise((resolve,reject) => {
  const timer=setTimeout(()=>reject(failure('FIXTURE')),10000);
  lines.once('line',line=>{ clearTimeout(timer); try {resolve(JSON.parse(line));} catch {reject(failure('FIXTURE'));} });
  signer.once('error',()=>{clearTimeout(timer);reject(failure('FIXTURE'));});
});
function sign(message) {
  return new Promise((resolve,reject)=> {
    const id=++signingID;
    const timer=setTimeout(()=>{waiting.delete(id);reject(failure('TIMEOUT'));},10000);
    waiting.set(id,{resolve,reject,timer}); signer.stdin.write(JSON.stringify({id,message})+'\n');
  });
}
async function evaluate(session, fn, ...args) {
  let operation = 'NONE';
  try {
    operation = 'EVALUATE_01';
    const result=await cdpCall('Runtime.evaluate',{expression:`(${fn.toString()})(...${JSON.stringify(args)})`,awaitPromise:true,returnByValue:true},session);
    operation = 'EVALUATE_02';
    if(result.exceptionDetails) throw new Error('Browser assertion failed');
    operation = 'EVALUATE_03';
    return result.result.value;
  } catch (error) {
    if (error instanceof Error) operations.set(error, [operation, ...(operations.get(error) ?? [])]);
    throw error;
  }
}
async function wait(session, fn) {
  let operation = 'NONE';
  try {
    operation = 'WAIT_01';
    const deadline=Date.now()+20000;
    operation = 'WAIT_02';
    while(Date.now()<deadline) { if(await evaluate(session,fn)) return; await new Promise(resolve=>setTimeout(resolve,50)); }
    operation = 'WAIT_03';
    throw failure('TIMEOUT');
  } catch (error) {
    if (error instanceof Error) operations.set(error, [operation, ...(operations.get(error) ?? [])]);
    throw error;
  }
}
async function click(session,text) {
  let operation = 'NONE';
  try {
    operation = 'CLICK_01';
    assert.equal(await evaluate(session,text=> { const button=[...document.querySelectorAll('button')].find(b=>b.textContent===text); if(!button||button.disabled)return false; button.click();return true; },text),true);
  } catch (error) {
    if (error instanceof Error) operations.set(error, [operation, ...(operations.get(error) ?? [])]);
    throw error;
  }
}
async function api(session,path,body,csrf) {
  return evaluate(session,async(path,body,csrf,post)=> {
    const r=await fetch(path,{method:post?'POST':'GET',credentials:'same-origin',cache:'no-store',redirect:'error',headers:post?{'Content-Type':'application/json',...(csrf?{'X-CSRF-Token':csrf}:{})}:{},body:post?JSON.stringify(body):undefined});
    return {status:r.status,body:await r.json()};
  },path,body??null,csrf??null,body!==undefined);
}

async function tab(url,address) {
  let operation = 'NONE';
  try {
    operation = 'TAB_01';
    const {targetId}=await cdpCall('Target.createTarget',{url:'about:blank',browserContextId:context});
    operation = 'TAB_02';
    const {sessionId}=await cdpCall('Target.attachToTarget',{targetId,flatten:true});
    operation = 'TAB_03';
    await cdpCall('Page.enable',{},sessionId); await cdpCall('Runtime.enable',{},sessionId);
    operation = 'TAB_04';
    await cdpCall('Network.enable',{},sessionId);
    operation = 'TAB_05';
    await cdpCall('Runtime.addBinding',{name:'__wp3dSign'},sessionId);
    operation = 'TAB_06';
    await cdpCall('Page.addScriptToEvaluateOnNewDocument',{source:walletScript(address)},sessionId);
    operation = 'TAB_07';
    const responses = new Set();
    cdp.on(event=> {
      if(event.sessionId===sessionId&&event.method==='Network.responseReceived'&&event.params.response.url===origin+'/browser/siwe/challenge'&&event.params.response.status===200) responses.add(event.params.requestId);
      if(event.sessionId===sessionId&&event.method==='Network.loadingFinished'&&responses.delete(event.params.requestId)) {
        void cdpCall('Network.getResponseBody',{requestId:event.params.requestId},sessionId).then(r=>challenges.set(sessionId,JSON.parse(r.body))).catch(()=>{});
      }
      if(event.sessionId!==sessionId||event.method!=='Runtime.bindingCalled'||event.params.name!=='__wp3dSign')return;
      void (async()=> {
        const input=JSON.parse(event.params.payload);
        assert.equal(input.address,address); assert.ok(Number.isSafeInteger(input.id));
        const signature=await sign(input.message);
        await evaluate(sessionId,(id,sig)=>window.__wp3dWalletResult(id,sig),input.id,signature);
      })().catch(()=>{ void evaluate(sessionId,id=>window.__wp3dWalletResult(id,null,true),JSON.parse(event.params.payload).id).catch(()=>{}); });
    });
    operation = 'TAB_08';
    await cdpCall('Page.navigate',{url},sessionId);
    operation = 'TAB_09';
    return sessionId;
  } catch (error) {
    if (error instanceof Error) operations.set(error, [operation, ...(operations.get(error) ?? [])]);
    throw error;
  }
}
const verifier=randomBytes(32).toString('base64url'), state=randomBytes(24).toString('base64url');
function authorize(f) {
  return origin+'/oauth/authorize?'+new URLSearchParams({client_id:f.client_id,redirect_uri:origin+'/browser-e2e/callback',resource,scope:'mcp:read',response_type:'code',code_challenge_method:'S256',code_challenge:createHash('sha256').update(verifier).digest('base64url'),state});
}
async function connect(session) {
  let operation = 'NONE';
  try {
    operation = 'CONNECT_01';
    await wait(session,()=>document.querySelector('select')?.options.length>1&&!document.querySelector('select').disabled);
    operation = 'CONNECT_02';
    await evaluate(session,()=>{ const select=document.querySelector('select'); select.value='550e8400-e29b-41d4-a716-446655440000';select.dispatchEvent(new Event('change',{bubbles:true})); });
    operation = 'CONNECT_03';
    await wait(session,()=>[...document.querySelectorAll('button')].some(b=>b.textContent==='Connect wallet'&&!b.disabled));
    operation = 'CONNECT_04';
    await click(session,'Connect wallet');
    operation = 'CONNECT_05';
    await wait(session,()=>[...document.querySelectorAll('button')].some(b=>b.textContent==='Request authentication message'&&!b.disabled));
  } catch (error) {
    if (error instanceof Error) operations.set(error, [operation, ...(operations.get(error) ?? [])]);
    throw error;
  }
}
async function authenticate(session,pending) {
  let operation = 'NONE';
  try {
    operation = 'AUTHENTICATE_01';
    challenges.delete(session);
    operation = 'AUTHENTICATE_02';
    await click(session,'Request authentication message');
    operation = 'AUTHENTICATE_03';
    await wait(session,()=>!!document.querySelector('pre'));
    operation = 'AUTHENTICATE_04';
    assert.equal(await evaluate(session,()=>[...document.querySelectorAll('button')].find(b=>b.textContent==='Grant read-only access').disabled),true);
    operation = 'AUTHENTICATE_05';
    await click(session,'Sign message to authenticate');
    operation = 'AUTHENTICATE_06';
    await wait(session,()=>[...document.querySelectorAll('button')].some(b=>b.textContent==='Grant read-only access'&&!b.disabled));
    operation = 'AUTHENTICATE_07';
    const current=await api(session,'/browser/session');
    operation = 'AUTHENTICATE_08';
    assert.equal(current.body.state,'AUTHENTICATED');
    operation = 'AUTHENTICATE_09';
    assert.equal(current.body.transaction_id,pending.transaction_id);
    operation = 'AUTHENTICATE_10';
    assert.ok(challenges.get(session));
    operation = 'AUTHENTICATE_11';
    assert.equal(current.body.challenge_id,challenges.get(session).challenge_id);
    operation = 'AUTHENTICATE_12';
    assert.notEqual(current.body.csrf,pending.csrf);
    operation = 'AUTHENTICATE_13';
    return current.body;
  } catch (error) {
    if (error instanceof Error) operations.set(error, [operation, ...(operations.get(error) ?? [])]);
    throw error;
  }
}
async function pendingTab(fixture,address=fixture.address) {
  let operation = 'NONE';
  try {
    operation = 'PENDINGTAB_01';
    const session=await tab(authorize(fixture),address);
    operation = 'PENDINGTAB_02';
    await wait(session,()=>location.pathname==='/onboarding'&&!!window.__wp3dWallet&&!!document.querySelector('select'));
    operation = 'PENDINGTAB_03';
    return session;
  } catch (error) {
    if (error instanceof Error) operations.set(error, [operation, ...(operations.get(error) ?? [])]);
    throw error;
  }
}
async function negatives(fixture) {
  // These are real DOM interactions and requests; only the wallet is synthetic.
  checkpoint('INVALID_OAUTH_REDIRECT', '01');
  const invalid=await tab(authorize(fixture).replace(encodeURIComponent(origin+'/browser-e2e/callback'),encodeURIComponent('https://attacker.invalid/callback')),fixture.address);
  checkpoint('INVALID_OAUTH_REDIRECT', '02');
  await wait(invalid,()=>location.pathname==='/oauth/authorize'&&document.readyState==='complete');
  checkpoint('INVALID_OAUTH_REDIRECT', '03');
  assert.equal(await evaluate(invalid,async()=>{const r=await fetch(location.href,{redirect:'error'});return r.status;}),400);
  checkpoint('AUTHORIZATION_DENIAL', '01');
  const denied=await pendingTab(fixture);
  checkpoint('AUTHORIZATION_DENIAL', '02');
  await click(denied,'Deny access');
  checkpoint('AUTHORIZATION_DENIAL', '03');
  await wait(denied,()=>document.querySelector('[role=status]')?.textContent.includes('Authorization denied'));
  checkpoint('AUTHORIZATION_DENIAL', '04');
  assert.equal((await api(denied,'/browser/session')).body.state,'DENIED');
  checkpoint('REJECTED_WALLET_SIGNING', '01');
  const rejected=await pendingTab(fixture);await connect(rejected);
  checkpoint('REJECTED_WALLET_SIGNING', '02');
  await click(rejected,'Request authentication message');await wait(rejected,()=>!!document.querySelector('pre'));
  checkpoint('REJECTED_WALLET_SIGNING', '03');
  await evaluate(rejected,()=>{window.__wp3dWallet.state.rejectSign=true;});await click(rejected,'Sign message to authenticate');
  checkpoint('REJECTED_WALLET_SIGNING', '04');
  await wait(rejected,()=>document.querySelector('[role=status]')?.textContent.includes('rejected'));
  checkpoint('REJECTED_WALLET_SIGNING', '05');
  assert.equal((await api(rejected,'/browser/session')).body.state,'PENDING');
  checkpoint('WRONG_NETWORK', '01');
  const wrongNetwork=await pendingTab(fixture);
  checkpoint('WRONG_NETWORK', '02');
  await evaluate(wrongNetwork,()=>{window.__wp3dWallet.state.chain='0x1';});
  checkpoint('WRONG_NETWORK', '03');
  await connect(wrongNetwork);
  checkpoint('WRONG_NETWORK', '04');
  await wait(wrongNetwork,()=>document.querySelector('[role=status]')?.textContent.includes('Wrong network'));
  checkpoint('WRONG_NETWORK', '05');
  await click(wrongNetwork,'Request authentication message');
  checkpoint('WRONG_NETWORK', '06');
  await wait(wrongNetwork,()=>document.querySelector('[role=status]')?.textContent.includes('Wrong network'));
  checkpoint('WRONG_NETWORK', '07');
  assert.equal(await evaluate(wrongNetwork,()=>!!document.querySelector('pre')),false);
  checkpoint('WRONG_NETWORK', '08');
  assert.equal(await evaluate(wrongNetwork,()=>window.__wp3dWallet.state.calls.filter(x=>x==='personal_sign').length),0);
  for(const [event, eventStage] of [['accountsChanged','ACCOUNTS_CHANGED'],['chainChanged','CHAIN_CHANGED'],['disconnect','DISCONNECT']]) {
    walletEvent = eventStage;
    checkpoint('WALLET_EVENT_INVALIDATION', '01');
    const session=await pendingTab(fixture);await connect(session);
    checkpoint('WALLET_EVENT_INVALIDATION', '02');
    await click(session,'Request authentication message');await wait(session,()=>!!document.querySelector('pre'));
    checkpoint('WALLET_EVENT_INVALIDATION', '03');
    await evaluate(session,()=>{window.__wp3dWallet.state.delay=true;});await click(session,'Sign message to authenticate');
    checkpoint('WALLET_EVENT_INVALIDATION', '04');
    await wait(session,()=>typeof window.__wp3dWallet.state.release==='function');
    checkpoint('WALLET_EVENT_INVALIDATION', '05');
    await evaluate(session,event=>{if(event==='chainChanged')window.__wp3dWallet.state.chain='0x1';window.__wp3dWallet.emit(event,event==='accountsChanged'?[]:'0x1');window.__wp3dWallet.state.release();},event);
    checkpoint('WALLET_EVENT_INVALIDATION', '06');
    await wait(session,()=>document.querySelector('[role=status]')?.textContent.includes('Restart'));
    checkpoint('WALLET_EVENT_INVALIDATION', '07');
    assert.equal(await evaluate(session,()=>{const button=[...document.querySelectorAll('button')].find(b=>b.textContent==='Grant read-only access');return !button||button.disabled;}),true);
    checkpoint('WALLET_EVENT_INVALIDATION', '08');
    assert.notEqual((await api(session,'/browser/session')).body.state,'AUTHENTICATED');
  }
  walletEvent = 'NONE';
  if(process.env.WP3D_BROWSER_EXPIRY==='yes') {
    checkpoint('CHALLENGE_EXPIRY', '01');
    const expired=await pendingTab(fixture), p=(await api(expired,'/browser/session')).body;
    checkpoint('CHALLENGE_EXPIRY', '02');
    const issued=await api(expired,'/browser/siwe/challenge',{address:fixture.address},p.csrf);
    checkpoint('CHALLENGE_EXPIRY', '03');
    assert.equal(issued.status,200);
    checkpoint('CHALLENGE_EXPIRY', '04');
    const signature=await sign(issued.body.message);
    checkpoint('CHALLENGE_EXPIRY', '05');
    await new Promise(resolve=>setTimeout(resolve,Math.max(0,Date.parse(issued.body.expires_at)-Date.now())+1000));
    checkpoint('CHALLENGE_EXPIRY', '06');
    assert.equal((await api(expired,'/browser/siwe/verify',{challenge_id:issued.body.challenge_id,signature},p.csrf)).status,403);
    checkpoint('CHALLENGE_EXPIRY', '07');
    assert.equal((await api(expired,'/browser/session')).body.state,'PENDING');
  }
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '01');
  const a=await pendingTab(fixture), pa=(await api(a,'/browser/session')).body;await connect(a);const aa=await authenticate(a,pa);
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '02');
  const b=await pendingTab(fixture,fixture.other_address), pb=(await api(b,'/browser/session')).body;await connect(b);const ab=await authenticate(b,pb);
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '03');
  assert.equal(ab.user_id,fixture.other_user_id);
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '04');
  assert.notEqual(aa.user_id,ab.user_id);
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '05');
  assert.notEqual(aa.challenge_id,ab.challenge_id);
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '06');
  assert.notEqual(aa.transaction_id,ab.transaction_id);
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '07');
  assert.equal((await api(a,'/browser/consent',{decision:'grant'},aa.csrf)).status,403);
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '08');
  await click(a,'Grant read-only access');
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '09');
  await wait(a,()=>document.querySelector('[role=status]')?.textContent.includes('uncertain'));
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '10');
  assert.equal((await api(b,'/browser/session')).body.challenge_id,ab.challenge_id);
  checkpoint('MULTI_TAB_SESSION_ISOLATION', '11');
  assert.equal(await evaluate(a,()=>[...document.querySelectorAll('button')].every(b=>b.textContent!=='Grant read-only access'||b.disabled)),true);
  checkpoint('INTERRUPTED_SIWE_TRANSPORT', '01');
  const interrupted=await pendingTab(fixture);await connect(interrupted);
  checkpoint('INTERRUPTED_SIWE_TRANSPORT', '02');
  await click(interrupted,'Request authentication message');await wait(interrupted,()=>!!document.querySelector('pre'));
  checkpoint('INTERRUPTED_SIWE_TRANSPORT', '03');
  let committedResponse=false;
  const detach=cdp.on(event=> {
    if(event.sessionId!==interrupted||event.method!=='Fetch.requestPaused')return;
    // Response-stage fault injection only AFTER actual server verification returned
    // 200. Never replace an authentication response or manufacture success.
    if(event.params.responseStatusCode===200) committedResponse=true;
    void cdpCall('Fetch.failRequest',{requestId:event.params.requestId,errorReason:'Aborted'},interrupted).catch(()=>{});
  });
  checkpoint('INTERRUPTED_SIWE_TRANSPORT', '04');
  await cdpCall('Fetch.enable',{patterns:[{urlPattern:origin+'/browser/siwe/verify',requestStage:'Response'}]},interrupted);
  checkpoint('INTERRUPTED_SIWE_TRANSPORT', '05');
  await click(interrupted,'Sign message to authenticate');
  checkpoint('INTERRUPTED_SIWE_TRANSPORT', '06');
  await wait(interrupted,()=>document.querySelector('[role=status]')?.textContent.includes('Restart'));
  checkpoint('INTERRUPTED_SIWE_TRANSPORT', '07');
  assert.equal(committedResponse,true);
  checkpoint('INTERRUPTED_SIWE_TRANSPORT', '08');
  assert.equal(await evaluate(interrupted,()=>[...document.querySelectorAll('button')].every(b=>b.textContent!=='Grant read-only access'||b.disabled)),true);
  checkpoint('INTERRUPTED_SIWE_TRANSPORT', '09');
  await cdpCall('Fetch.disable',{},interrupted);detach();
  console.log('PASS: real-browser denial, rejected/delayed signing, wallet events, multi-tab rejection and interrupted verification.');
}
try {
  checkpoint('FIXTURE_INITIALIZATION', '01');
  const fixture=await first;
  checkpoint('FIXTURE_INITIALIZATION', '02');
  lines.on('line',line=>{ const r=JSON.parse(line), p=waiting.get(r.id); if(!p)return;waiting.delete(r.id);clearTimeout(p.timer);r.error?p.reject(failure('FIXTURE')):p.resolve(r.signature); });
  checkpoint('FIXTURE_INITIALIZATION', '03');
  await categorized('FIXTURE', () => mkdir('deploy/local/runtime',{recursive:true,mode:0o700}));
  checkpoint('FIXTURE_INITIALIZATION', '04');
  await categorized('FIXTURE', () => writeFile('deploy/local/runtime/browser-e2e-fixture.sql',fixture.sql,{mode:0o600}));
  checkpoint('FIXTURE_INITIALIZATION', '05');
  await categorized('FIXTURE', () => writeFile('deploy/local/runtime/browser-e2e-public.json',JSON.stringify({tenant_id:fixture.tenant_id,user_id:fixture.user_id,client_id:fixture.client_id,address:fixture.address}),{mode:0o600}));
  console.log('Public synthetic fixture prepared. Apply it ONLY to wizpay_mcp_browser_e2e and configure the test Go service with tenant_id from browser-e2e-public.json.');
  console.log('After approved setup and trusted browser launch, enter READY. No key is stored; keep this process alive.');
  checkpoint('FIXTURE_INITIALIZATION', '06');
  const input=createInterface({input:process.stdin});
  checkpoint('FIXTURE_INITIALIZATION', '07');
  assert.equal(await new Promise(resolve=>input.once('line',resolve)),'READY');input.close();
  checkpoint('CDP_CONNECTION', '01');
  cdp=await categorized('CDP', () => CDP.connect(process.env.WP3D_BROWSER_CDP));
  checkpoint('CHROME_POLICY_VALIDATION', '01');
  const browserArguments=(await cdpCall('Browser.getBrowserCommandLine')).arguments;
  await categorized('CDP', () => validateBrowserArguments(browserArguments));
  checkpoint('BROWSER_CONTEXT_INITIALIZATION', '01');
  const {browserContextId}=await cdpCall('Target.createBrowserContext');context=browserContextId;
  checkpoint('BROWSER_CONTEXT_INITIALIZATION', '02');
  await negatives(fixture);
  checkpoint('OAUTH_AUTHORIZATION_ONBOARDING', '01');
  const a=await tab(authorize(fixture),fixture.address);
  checkpoint('OAUTH_AUTHORIZATION_ONBOARDING', '02');
  await wait(a,()=>location.pathname==='/onboarding'&&!!window.__wp3dWallet&&!!document.querySelector('select'));
  checkpoint('OAUTH_AUTHORIZATION_ONBOARDING', '03');
  assert.equal(await evaluate(a,()=>window.isSecureContext),true);
  checkpoint('OAUTH_AUTHORIZATION_ONBOARDING', '04');
  const pending=(await api(a,'/browser/session')).body;
  checkpoint('OAUTH_AUTHORIZATION_ONBOARDING', '05');
  assert.equal(pending.state,'PENDING');
  checkpoint('OAUTH_AUTHORIZATION_ONBOARDING', '06');
  assert.equal(pending.authentication_available,true);
  checkpoint('SECURE_COOKIE_CSRF_VALIDATION', '01');
  const cookies=(await cdpCall('Storage.getCookies',{browserContextId:context})).cookies;
  checkpoint('SECURE_COOKIE_CSRF_VALIDATION', '02');
  const old=cookies.find(c=>c.name==='__Host-wizpay-browser');
  checkpoint('SECURE_COOKIE_CSRF_VALIDATION', '03');
  assert.ok(old);
  checkpoint('SECURE_COOKIE_CSRF_VALIDATION', '04');
  assert.equal(old.domain,'connect.wizpay.xyz');
  checkpoint('SECURE_COOKIE_CSRF_VALIDATION', '05');
  assert.equal(old.path,'/');
  checkpoint('SECURE_COOKIE_CSRF_VALIDATION', '06');
  assert.equal(old.secure,true);
  checkpoint('SECURE_COOKIE_CSRF_VALIDATION', '07');
  assert.equal(old.httpOnly,true);
  checkpoint('SECURE_COOKIE_CSRF_VALIDATION', '08');
  assert.equal(old.sameSite,'Lax');
  checkpoint('SECURE_COOKIE_CSRF_VALIDATION', '09');
  assert.equal(await evaluate(a,()=>document.cookie.includes('__Host-wizpay-browser')),false);
  checkpoint('SECURE_COOKIE_CSRF_VALIDATION', '10');
  assert.equal((await api(a,'/browser/consent',{decision:'grant'})).status,403);
  checkpoint('WALLET_CONNECTION_SIWE_AUTHENTICATION', '01');
  await connect(a);
  checkpoint('WALLET_CONNECTION_SIWE_AUTHENTICATION', '02');
  const auth=await authenticate(a,pending);
  checkpoint('WALLET_CONNECTION_SIWE_AUTHENTICATION', '03');
  assert.equal(auth.user_id,fixture.user_id);
  checkpoint('SIWE_REPLAY_REJECTION', '01');
  const replay=await sign(challenges.get(a).message);
  checkpoint('SIWE_REPLAY_REJECTION', '02');
  assert.equal((await api(a,'/browser/siwe/verify',{challenge_id:auth.challenge_id,signature:replay},auth.csrf)).status,403);
  checkpoint('SESSION_ROTATION_STALE_COOKIE_REJECTION', '01');
  const replacement=(await cdpCall('Storage.getCookies',{browserContextId:context})).cookies.find(c=>c.name===old.name);
  checkpoint('SESSION_ROTATION_STALE_COOKIE_REJECTION', '02');
  assert.notEqual(replacement.value,old.value);
  checkpoint('SESSION_ROTATION_STALE_COOKIE_REJECTION', '03');
  assert.equal((await api(a,'/browser/consent',{decision:'grant'},pending.csrf)).status,403);
  checkpoint('SESSION_ROTATION_STALE_COOKIE_REJECTION', '04');
  await cdpCall('Storage.setCookies',{browserContextId:context,cookies:[{name:old.name,value:old.value,url:origin,path:'/',secure:true,httpOnly:true,sameSite:'Lax'}]});
  checkpoint('SESSION_ROTATION_STALE_COOKIE_REJECTION', '05');
  assert.equal((await api(a,'/browser/session')).status,401);
  checkpoint('SESSION_ROTATION_STALE_COOKIE_REJECTION', '06');
  await cdpCall('Storage.setCookies',{browserContextId:context,cookies:[{name:replacement.name,value:replacement.value,url:origin,path:'/',secure:true,httpOnly:true,sameSite:'Lax'}]});
  // Reload uses real persisted session authority, not wallet connection status.
  checkpoint('CONSENT_OAUTH_CALLBACK', '01');
  await cdpCall('Page.reload',{},a);await wait(a,()=>[...document.querySelectorAll('button')].some(b=>b.textContent==='Grant read-only access'&&!b.disabled));
  checkpoint('CONSENT_OAUTH_CALLBACK', '02');
  await click(a,'Grant read-only access');
  checkpoint('CONSENT_OAUTH_CALLBACK', '03');
  await wait(a,()=>location.pathname==='/browser-e2e/callback');
  checkpoint('CONSENT_OAUTH_CALLBACK', '04');
  const callback=new URL(await evaluate(a,()=>location.href));
  checkpoint('CONSENT_OAUTH_CALLBACK', '05');
  assert.equal(callback.origin,origin);
  checkpoint('CONSENT_OAUTH_CALLBACK', '06');
  assert.equal(callback.searchParams.get('state'),state);
  checkpoint('CONSENT_OAUTH_CALLBACK', '07');
  assert.equal(callback.searchParams.get('iss'),origin);
  checkpoint('CONSENT_OAUTH_CALLBACK', '08');
  assert.ok(callback.searchParams.get('code'));
  checkpoint('PKCE_TOKEN_EXCHANGE', '01');
  const tokenForm=new URLSearchParams({grant_type:'authorization_code',client_id:fixture.client_id,redirect_uri:origin+'/browser-e2e/callback',resource,code:callback.searchParams.get('code'),code_verifier:verifier}).toString();
  const tokenCA=await readFile('deploy/local/runtime/browser-tls/ca.crt');
  const token=await new Promise((resolve,reject)=> {
    // AI-client transport: no browser headers; canonical Host/SNI and verified TLS.
    // A dedicated connection and fixed lookup prevent external DNS/agent routing.
    const request=httpsRequest(origin+'/oauth/token',{
      method:'POST',port:443,agent:false,ca:tokenCA,rejectUnauthorized:true,
      servername:'connect.wizpay.xyz',family:4,
      lookup:(_hostname,options,done)=>done(null,...(options.all?[[{address:'127.0.0.1',family:4}]]:['127.0.0.1',4])),
      headers:{'Content-Type':'application/x-www-form-urlencoded','Content-Length':Buffer.byteLength(tokenForm)},
    },async response=> {
      try {
        const chunks=[];
        for await (const chunk of response) chunks.push(chunk);
        resolve({status:response.statusCode,body:JSON.parse(Buffer.concat(chunks).toString('utf8'))});
      } catch(error) {reject(error);}
    });
    request.on('error',reject);
    request.setTimeout(20000,()=>request.destroy(failure('TIMEOUT')));
    request.end(tokenForm);
  });
  checkpoint('PKCE_TOKEN_EXCHANGE', '02');
  assert.equal(token.status,200);
  checkpoint('PKCE_TOKEN_EXCHANGE', '03');
  assert.equal(token.body.scope,'mcp:read');
  checkpoint('MCP_ORIGIN_COOKIE_ISOLATION', '01');
  const m=await tab('https://mcp.wizpay.xyz/.well-known/oauth-protected-resource/mcp',fixture.address);
  checkpoint('MCP_ORIGIN_COOKIE_ISOLATION', '02');
  await wait(m,()=>location.hostname==='mcp.wizpay.xyz'&&document.readyState==='complete');
  checkpoint('MCP_ORIGIN_COOKIE_ISOLATION', '03');
  assert.equal((await cdpCall('Network.getCookies',{urls:['https://mcp.wizpay.xyz/mcp']},m)).cookies.some(c=>c.name===old.name),false);
  checkpoint('MCP_ORIGIN_COOKIE_ISOLATION', '04');
  const rpc=async(method,params,bearer=token.body.access_token)=>evaluate(m,async(method,params,bearer)=>{const r=await fetch('/mcp',{method:'POST',headers:{'Content-Type':'application/json',Accept:'application/json, text/event-stream',...(bearer?{Authorization:'Bearer '+bearer}:{})},body:JSON.stringify({jsonrpc:'2.0',id:1,method,params})});return {status:r.status,challenge:r.headers.get('WWW-Authenticate'),body:await r.json()};},method,params,bearer);
  checkpoint('MCP_INITIALIZATION', '01');
  assert.equal((await rpc('initialize',{protocolVersion:'2025-11-25',capabilities:{},clientInfo:{name:'synthetic-browser',version:'1'}})).status,200);
  checkpoint('MCP_TOOL_DISCOVERY', '01');
  const listed=await rpc('tools/list',{});
  checkpoint('MCP_TOOL_DISCOVERY', '02');
  assert.deepEqual(listed.body.result.tools.map(t=>t.name).sort(),['wizpay.get_approval','wizpay.get_intent']);
  for(const name of ['wizpay.create_intent','wizpay.request_approval','wizpay.evaluate_policy','wizpay.prepare_execution','wizpay.send.preview','wizpay.send.create_intent','wizpay.send.execute','wizpay.send.status','wizpay.payroll.preview','wizpay.payroll.create_intent','wizpay.payroll.execute','wizpay.payroll.status','wizpay.swap.preview','wizpay.swap.create_intent','wizpay.swap.execute','wizpay.swap.status','wizpay.autonomy.list_schedules','wizpay.autonomy.get_schedule','wizpay.autonomy.simulate','wizpay.autonomy.create_schedule','wizpay.autonomy.control_schedule','wizpay.autonomy.emergency_stop']) {
    excludedTool = excludedToolIDs.get(name);
    checkpoint('EXCLUDED_TOOL_REJECTION', '01');
    const denied=await rpc('tools/call',{name,arguments:{}});
    checkpoint('EXCLUDED_TOOL_REJECTION', '02');
    assert.equal(denied.body.error.code,-32602);
  }
  excludedTool = 'NONE';
  // Missing bearer is inspected without assuming a JSON body on a 401.
  checkpoint('MISSING_BEARER_REJECTION', '01');
  const missing=await evaluate(m,async()=>{const r=await fetch('/mcp');return {status:r.status,challenge:r.headers.get('WWW-Authenticate')};});
  checkpoint('MISSING_BEARER_REJECTION', '02');
  assert.equal(missing.status,401);
  checkpoint('MISSING_BEARER_REJECTION', '03');
  assert.ok(missing.challenge.includes('resource_metadata'));
  checkpoint('INVALID_BEARER_REJECTION', '01');
  const invalidBearer=await evaluate(m,async()=>{const r=await fetch('/mcp',{headers:{Authorization:'Bearer invalid'}});return {status:r.status,challenge:r.headers.get('WWW-Authenticate')};});
  checkpoint('INVALID_BEARER_REJECTION', '02');
  assert.equal(invalidBearer.status,401);
  checkpoint('INVALID_BEARER_REJECTION', '03');
  assert.ok(invalidBearer.challenge.includes('invalid_token'));
  // Completed consent is terminal: session presentation rejects the completed
  // transaction. Logout still accepts the current cookie and rotation-era CSRF.
  checkpoint('LOGOUT_SESSION_INVALIDATION', '01');
  assert.equal((await api(a,'/browser/session')).status,401);
  checkpoint('LOGOUT_SESSION_INVALIDATION', '02');
  const logout=await evaluate(a,async csrf=>{const r=await fetch('/browser/logout',{method:'POST',headers:{'X-CSRF-Token':csrf}});return r.status;},auth.csrf);
  checkpoint('LOGOUT_SESSION_INVALIDATION', '03');
  assert.equal(logout,200);
  checkpoint('LOGOUT_SESSION_INVALIDATION', '04');
  assert.equal((await api(a,'/browser/session')).status,401);
  checkpoint('LOGOUT_SESSION_INVALIDATION', '05');
  assert.equal((await cdpCall('Storage.getCookies',{browserContextId:context})).cookies.some(c=>c.name===old.name),false);
  console.log('PASS: real-browser HTTPS continuous OAuth/SIWE/rotation/consent/PKCE/MCP/logout checks.');
} catch (error) {
  reportFailure(error);process.exitCode=1;
} finally {
  if(cdp) {if(context)await cdp.call('Target.disposeBrowserContext',{browserContextId:context}).catch(()=>{});cdp.close();}
  for(const p of waiting.values()){clearTimeout(p.timer);p.reject(new Error('Fixture ended'));}signer.kill();lines.close();
}
