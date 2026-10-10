// Owner-run only. Requires trusted local HTTPS, current isolated stack and Chrome.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { mkdir, writeFile } from 'node:fs/promises';
import { randomBytes, createHash } from 'node:crypto';
import { CDP, validateBrowserArguments } from './cdp.mjs';
import { walletScript } from './wallet.mjs';

const origin = 'https://connect.wizpay.xyz', resource = 'https://mcp.wizpay.xyz/mcp';
if (process.env.WP3D_BROWSER_OWNER_APPROVED !== 'yes') throw new Error('Owner approval for isolated browser execution required');
const signer = spawn('./deploy/local/runtime/browser-e2e-signer', [], {stdio:['pipe','pipe','ignore']});
const lines = createInterface({input:signer.stdout});
const waiting = new Map(), challenges = new Map(); let signingID=0, cdp, context;
const first = new Promise((resolve,reject) => {
  const timer=setTimeout(()=>reject(new Error('Fixture startup failed')),10000);
  lines.once('line',line=>{ clearTimeout(timer); try {resolve(JSON.parse(line));} catch {reject(new Error('Fixture startup failed'));} });
  signer.once('error',()=>{clearTimeout(timer);reject(new Error('Fixture startup failed'));});
});
function sign(message) {
  return new Promise((resolve,reject)=> {
    const id=++signingID;
    const timer=setTimeout(()=>{waiting.delete(id);reject(new Error('Synthetic signing timed out'));},10000);
    waiting.set(id,{resolve,reject,timer}); signer.stdin.write(JSON.stringify({id,message})+'\n');
  });
}
async function evaluate(session, fn, ...args) {
  const result=await cdp.call('Runtime.evaluate',{expression:`(${fn.toString()})(...${JSON.stringify(args)})`,awaitPromise:true,returnByValue:true},session);
  if(result.exceptionDetails) throw new Error('Browser assertion failed');
  return result.result.value;
}
async function wait(session, fn) {
  const deadline=Date.now()+20000;
  while(Date.now()<deadline) { if(await evaluate(session,fn)) return; await new Promise(resolve=>setTimeout(resolve,50)); }
  throw new Error('Browser state timed out');
}
async function click(session,text) {
  assert.equal(await evaluate(session,text=> { const button=[...document.querySelectorAll('button')].find(b=>b.textContent===text); if(!button||button.disabled)return false; button.click();return true; },text),true);
}
async function api(session,path,body,csrf) {
  return evaluate(session,async(path,body,csrf,post)=> {
    const r=await fetch(path,{method:post?'POST':'GET',credentials:'same-origin',cache:'no-store',redirect:'error',headers:post?{'Content-Type':'application/json',...(csrf?{'X-CSRF-Token':csrf}:{})}:{},body:post?JSON.stringify(body):undefined});
    return {status:r.status,body:await r.json()};
  },path,body??null,csrf??null,body!==undefined);
}
async function tab(url,address) {
  const {targetId}=await cdp.call('Target.createTarget',{url:'about:blank',browserContextId:context});
  const {sessionId}=await cdp.call('Target.attachToTarget',{targetId,flatten:true});
  await cdp.call('Page.enable',{},sessionId); await cdp.call('Runtime.enable',{},sessionId);
  await cdp.call('Network.enable',{},sessionId);
  await cdp.call('Runtime.addBinding',{name:'__wp3dSign'},sessionId);
  await cdp.call('Page.addScriptToEvaluateOnNewDocument',{source:walletScript(address)},sessionId);
  const responses = new Set();
  cdp.on(event=> {
    if(event.sessionId===sessionId&&event.method==='Network.responseReceived'&&event.params.response.url===origin+'/browser/siwe/challenge'&&event.params.response.status===200) responses.add(event.params.requestId);
    if(event.sessionId===sessionId&&event.method==='Network.loadingFinished'&&responses.delete(event.params.requestId)) {
      void cdp.call('Network.getResponseBody',{requestId:event.params.requestId},sessionId).then(r=>challenges.set(sessionId,JSON.parse(r.body))).catch(()=>{});
    }
    if(event.sessionId!==sessionId||event.method!=='Runtime.bindingCalled'||event.params.name!=='__wp3dSign')return;
    void (async()=> {
      const input=JSON.parse(event.params.payload);
      assert.equal(input.address,address); assert.ok(Number.isSafeInteger(input.id));
      const signature=await sign(input.message);
      await evaluate(sessionId,(id,sig)=>window.__wp3dWalletResult(id,sig),input.id,signature);
    })().catch(()=>{ void evaluate(sessionId,id=>window.__wp3dWalletResult(id,null,true),JSON.parse(event.params.payload).id).catch(()=>{}); });
  });
  await cdp.call('Page.navigate',{url},sessionId);
  return sessionId;
}
const verifier=randomBytes(32).toString('base64url'), state=randomBytes(24).toString('base64url');
function authorize(f) {
  return origin+'/oauth/authorize?'+new URLSearchParams({client_id:f.client_id,redirect_uri:origin+'/browser-e2e/callback',resource,scope:'mcp:read',response_type:'code',code_challenge_method:'S256',code_challenge:createHash('sha256').update(verifier).digest('base64url'),state});
}
async function connect(session) {
  await wait(session,()=>document.querySelector('select')?.options.length>1&&!document.querySelector('select').disabled);
  await evaluate(session,()=>{ const select=document.querySelector('select'); select.value='550e8400-e29b-41d4-a716-446655440000';select.dispatchEvent(new Event('change',{bubbles:true})); });
  await wait(session,()=>[...document.querySelectorAll('button')].some(b=>b.textContent==='Connect wallet'&&!b.disabled));
  await click(session,'Connect wallet');
  await wait(session,()=>[...document.querySelectorAll('button')].some(b=>b.textContent==='Request authentication message'&&!b.disabled));
}
async function authenticate(session,pending) {
  challenges.delete(session);
  await click(session,'Request authentication message');
  await wait(session,()=>!!document.querySelector('pre'));
  assert.equal(await evaluate(session,()=>[...document.querySelectorAll('button')].find(b=>b.textContent==='Grant read-only access').disabled),true);
  await click(session,'Sign message to authenticate');
  await wait(session,()=>[...document.querySelectorAll('button')].some(b=>b.textContent==='Grant read-only access'&&!b.disabled));
  const current=await api(session,'/browser/session');
  assert.equal(current.body.state,'AUTHENTICATED');assert.equal(current.body.transaction_id,pending.transaction_id);assert.ok(challenges.get(session));assert.equal(current.body.challenge_id,challenges.get(session).challenge_id);assert.notEqual(current.body.csrf,pending.csrf);
  return current.body;
}
async function pendingTab(fixture,address=fixture.address) {
  const session=await tab(authorize(fixture),address);
  await wait(session,()=>location.pathname==='/onboarding'&&!!window.__wp3dWallet&&!!document.querySelector('select'));
  return session;
}
async function negatives(fixture) {
  // These are real DOM interactions and requests; only the wallet is synthetic.
  const invalid=await tab(authorize(fixture).replace(encodeURIComponent(origin+'/browser-e2e/callback'),encodeURIComponent('https://attacker.invalid/callback')),fixture.address);
  await wait(invalid,()=>location.pathname==='/oauth/authorize'&&document.readyState==='complete');
  assert.equal(await evaluate(invalid,async()=>{const r=await fetch(location.href,{redirect:'error'});return r.status;}),400);
  const denied=await pendingTab(fixture);
  await click(denied,'Deny access');
  await wait(denied,()=>document.querySelector('[role=status]')?.textContent.includes('Authorization denied'));
  assert.equal((await api(denied,'/browser/session')).body.state,'DENIED');
  const rejected=await pendingTab(fixture);await connect(rejected);
  await click(rejected,'Request authentication message');await wait(rejected,()=>!!document.querySelector('pre'));
  await evaluate(rejected,()=>{window.__wp3dWallet.state.rejectSign=true;});await click(rejected,'Sign message to authenticate');
  await wait(rejected,()=>document.querySelector('[role=status]')?.textContent.includes('rejected'));
  assert.equal((await api(rejected,'/browser/session')).body.state,'PENDING');
  const wrongNetwork=await pendingTab(fixture);
  await evaluate(wrongNetwork,()=>{window.__wp3dWallet.state.chain='0x1';});
  await connect(wrongNetwork);
  await wait(wrongNetwork,()=>document.querySelector('[role=status]')?.textContent.includes('Wrong network'));
  await click(wrongNetwork,'Request authentication message');
  await wait(wrongNetwork,()=>document.querySelector('[role=status]')?.textContent.includes('Wrong network'));
  assert.equal(await evaluate(wrongNetwork,()=>!!document.querySelector('pre')),false);
  assert.equal(await evaluate(wrongNetwork,()=>window.__wp3dWallet.state.calls.filter(x=>x==='personal_sign').length),0);
  for(const event of ['accountsChanged','chainChanged','disconnect']) {
    const session=await pendingTab(fixture);await connect(session);
    await click(session,'Request authentication message');await wait(session,()=>!!document.querySelector('pre'));
    await evaluate(session,()=>{window.__wp3dWallet.state.delay=true;});await click(session,'Sign message to authenticate');
    await wait(session,()=>typeof window.__wp3dWallet.state.release==='function');
    await evaluate(session,event=>{if(event==='chainChanged')window.__wp3dWallet.state.chain='0x1';window.__wp3dWallet.emit(event,event==='accountsChanged'?[]:'0x1');window.__wp3dWallet.state.release();},event);
    await wait(session,()=>document.querySelector('[role=status]')?.textContent.includes('Restart'));
    assert.equal(await evaluate(session,()=>[...document.querySelectorAll('button')].find(b=>b.textContent==='Grant read-only access').disabled),true);
    assert.notEqual((await api(session,'/browser/session')).body.state,'AUTHENTICATED');
  }
  if(process.env.WP3D_BROWSER_EXPIRY==='yes') {
    const expired=await pendingTab(fixture), p=(await api(expired,'/browser/session')).body;
    const issued=await api(expired,'/browser/siwe/challenge',{address:fixture.address},p.csrf);assert.equal(issued.status,200);
    const signature=await sign(issued.body.message);
    await new Promise(resolve=>setTimeout(resolve,Math.max(0,Date.parse(issued.body.expires_at)-Date.now())+1000));
    assert.equal((await api(expired,'/browser/siwe/verify',{challenge_id:issued.body.challenge_id,signature},p.csrf)).status,403);
    assert.equal((await api(expired,'/browser/session')).body.state,'PENDING');
  }
  const a=await pendingTab(fixture), pa=(await api(a,'/browser/session')).body;await connect(a);const aa=await authenticate(a,pa);
  const b=await pendingTab(fixture,fixture.other_address), pb=(await api(b,'/browser/session')).body;await connect(b);const ab=await authenticate(b,pb);
  assert.equal(ab.user_id,fixture.other_user_id);assert.notEqual(aa.user_id,ab.user_id);
  assert.notEqual(aa.challenge_id,ab.challenge_id);assert.notEqual(aa.transaction_id,ab.transaction_id);
  assert.equal((await api(a,'/browser/consent',{decision:'grant'},aa.csrf)).status,403);
  await click(a,'Grant read-only access');
  await wait(a,()=>document.querySelector('[role=status]')?.textContent.includes('uncertain'));
  assert.equal((await api(b,'/browser/session')).body.challenge_id,ab.challenge_id);
  assert.equal(await evaluate(a,()=>[...document.querySelectorAll('button')].every(b=>b.textContent!=='Grant read-only access'||b.disabled)),true);
  const interrupted=await pendingTab(fixture);await connect(interrupted);
  await click(interrupted,'Request authentication message');await wait(interrupted,()=>!!document.querySelector('pre'));
  let committedResponse=false;
  const detach=cdp.on(event=> {
    if(event.sessionId!==interrupted||event.method!=='Fetch.requestPaused')return;
    // Response-stage fault injection only AFTER actual server verification returned
    // 200. Never replace an authentication response or manufacture success.
    if(event.params.responseStatusCode===200) committedResponse=true;
    void cdp.call('Fetch.failRequest',{requestId:event.params.requestId,errorReason:'Aborted'},interrupted).catch(()=>{});
  });
  await cdp.call('Fetch.enable',{patterns:[{urlPattern:origin+'/browser/siwe/verify',requestStage:'Response'}]},interrupted);
  await click(interrupted,'Sign message to authenticate');
  await wait(interrupted,()=>document.querySelector('[role=status]')?.textContent.includes('Restart'));
  assert.equal(committedResponse,true);
  assert.equal(await evaluate(interrupted,()=>[...document.querySelectorAll('button')].every(b=>b.textContent!=='Grant read-only access'||b.disabled)),true);
  await cdp.call('Fetch.disable',{},interrupted);detach();
  console.log('PASS: real-browser denial, rejected/delayed signing, wallet events, multi-tab rejection and interrupted verification.');
}
try {
  const fixture=await first;
  lines.on('line',line=>{ const r=JSON.parse(line), p=waiting.get(r.id); if(!p)return;waiting.delete(r.id);clearTimeout(p.timer);r.error?p.reject(new Error('Synthetic signature rejected')):p.resolve(r.signature); });
  await mkdir('deploy/local/runtime',{recursive:true,mode:0o700});
  await writeFile('deploy/local/runtime/browser-e2e-fixture.sql',fixture.sql,{mode:0o600});
  await writeFile('deploy/local/runtime/browser-e2e-public.json',JSON.stringify({tenant_id:fixture.tenant_id,user_id:fixture.user_id,client_id:fixture.client_id,address:fixture.address}),{mode:0o600});
  console.log('Public synthetic fixture prepared. Apply it ONLY to wizpay_mcp_browser_e2e and configure the test Go service with tenant_id from browser-e2e-public.json.');
  console.log('After approved setup and trusted browser launch, enter READY. No key is stored; keep this process alive.');
  const input=createInterface({input:process.stdin});
  assert.equal(await new Promise(resolve=>input.once('line',resolve)),'READY');input.close();
  cdp=await CDP.connect(process.env.WP3D_BROWSER_CDP);
  validateBrowserArguments((await cdp.call('Browser.getBrowserCommandLine')).arguments);
  const {browserContextId}=await cdp.call('Target.createBrowserContext');context=browserContextId;
  await negatives(fixture);
  const a=await tab(authorize(fixture),fixture.address);
  await wait(a,()=>location.pathname==='/onboarding'&&!!window.__wp3dWallet&&!!document.querySelector('select'));
  assert.equal(await evaluate(a,()=>window.isSecureContext),true);
  const pending=(await api(a,'/browser/session')).body;assert.equal(pending.state,'PENDING');assert.equal(pending.authentication_available,true);
  const cookies=(await cdp.call('Storage.getCookies',{browserContextId:context})).cookies;
  const old=cookies.find(c=>c.name==='__Host-wizpay-browser');
  assert.ok(old);assert.equal(old.domain,'connect.wizpay.xyz');assert.equal(old.path,'/');assert.equal(old.secure,true);assert.equal(old.httpOnly,true);assert.equal(old.sameSite,'Lax');
  assert.equal(await evaluate(a,()=>document.cookie.includes('__Host-wizpay-browser')),false);
  assert.equal((await api(a,'/browser/consent',{decision:'grant'})).status,403);
  await connect(a);const auth=await authenticate(a,pending);assert.equal(auth.user_id,fixture.user_id);
  const replay=await sign(challenges.get(a).message);
  assert.equal((await api(a,'/browser/siwe/verify',{challenge_id:auth.challenge_id,signature:replay},auth.csrf)).status,403);
  const replacement=(await cdp.call('Storage.getCookies',{browserContextId:context})).cookies.find(c=>c.name===old.name);
  assert.notEqual(replacement.value,old.value);
  assert.equal((await api(a,'/browser/consent',{decision:'grant'},pending.csrf)).status,403);
  await cdp.call('Storage.setCookies',{browserContextId:context,cookies:[{name:old.name,value:old.value,url:origin,path:'/',secure:true,httpOnly:true,sameSite:'Lax'}]});
  assert.equal((await api(a,'/browser/session')).status,401);
  await cdp.call('Storage.setCookies',{browserContextId:context,cookies:[{name:replacement.name,value:replacement.value,url:origin,path:'/',secure:true,httpOnly:true,sameSite:'Lax'}]});
  // Reload uses real persisted session authority, not wallet connection status.
  await cdp.call('Page.reload',{},a);await wait(a,()=>[...document.querySelectorAll('button')].some(b=>b.textContent==='Grant read-only access'&&!b.disabled));
  await click(a,'Grant read-only access');
  await wait(a,()=>location.pathname==='/browser-e2e/callback');
  const callback=new URL(await evaluate(a,()=>location.href));assert.equal(callback.origin,origin);assert.equal(callback.searchParams.get('state'),state);assert.equal(callback.searchParams.get('iss'),origin);assert.ok(callback.searchParams.get('code'));
  const token=await evaluate(a,async(f,v,code)=>{const r=await fetch('/oauth/token',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({grant_type:'authorization_code',client_id:f.client_id,redirect_uri:'https://connect.wizpay.xyz/browser-e2e/callback',resource:'https://mcp.wizpay.xyz/mcp',code,code_verifier:v})});return {status:r.status,body:await r.json()};},fixture,verifier,callback.searchParams.get('code'));
  assert.equal(token.status,200);assert.equal(token.body.scope,'mcp:read');
  const m=await tab('https://mcp.wizpay.xyz/.well-known/oauth-protected-resource/mcp',fixture.address);
  await wait(m,()=>location.hostname==='mcp.wizpay.xyz'&&document.readyState==='complete');
  assert.equal((await cdp.call('Network.getCookies',{urls:['https://mcp.wizpay.xyz/mcp']},m)).cookies.some(c=>c.name===old.name),false);
  const rpc=async(method,params,bearer=token.body.access_token)=>evaluate(m,async(method,params,bearer)=>{const r=await fetch('/mcp',{method:'POST',headers:{'Content-Type':'application/json',Accept:'application/json, text/event-stream',...(bearer?{Authorization:'Bearer '+bearer}:{})},body:JSON.stringify({jsonrpc:'2.0',id:1,method,params})});return {status:r.status,challenge:r.headers.get('WWW-Authenticate'),body:await r.json()};},method,params,bearer);
  assert.equal((await rpc('initialize',{protocolVersion:'2025-11-25',capabilities:{},clientInfo:{name:'synthetic-browser',version:'1'}})).status,200);
  const listed=await rpc('tools/list',{});assert.deepEqual(listed.body.result.tools.map(t=>t.name).sort(),['wizpay.get_approval','wizpay.get_intent']);
  for(const name of ['wizpay.create_intent','wizpay.request_approval','wizpay.evaluate_policy','wizpay.prepare_execution','wizpay.send.preview','wizpay.send.create_intent','wizpay.send.execute','wizpay.send.status','wizpay.payroll.preview','wizpay.payroll.create_intent','wizpay.payroll.execute','wizpay.payroll.status','wizpay.swap.preview','wizpay.swap.create_intent','wizpay.swap.execute','wizpay.swap.status','wizpay.autonomy.list_schedules','wizpay.autonomy.get_schedule','wizpay.autonomy.simulate','wizpay.autonomy.create_schedule','wizpay.autonomy.control_schedule','wizpay.autonomy.emergency_stop']) {const denied=await rpc('tools/call',{name,arguments:{}});assert.equal(denied.body.error.code,-32602);}
  // Missing bearer is inspected without assuming a JSON body on a 401.
  const missing=await evaluate(m,async()=>{const r=await fetch('/mcp');return {status:r.status,challenge:r.headers.get('WWW-Authenticate')};});assert.equal(missing.status,401);assert.ok(missing.challenge.includes('resource_metadata'));
  const invalidBearer=await evaluate(m,async()=>{const r=await fetch('/mcp',{headers:{Authorization:'Bearer invalid'}});return {status:r.status,challenge:r.headers.get('WWW-Authenticate')};});assert.equal(invalidBearer.status,401);assert.ok(invalidBearer.challenge.includes('invalid_token'));
  // Completed consent is terminal: session presentation rejects the completed
  // transaction. Logout still accepts the current cookie and rotation-era CSRF.
  assert.equal((await api(a,'/browser/session')).status,401);
  const logout=await evaluate(a,async csrf=>{const r=await fetch('/browser/logout',{method:'POST',headers:{'X-CSRF-Token':csrf}});return r.status;},auth.csrf);assert.equal(logout,200);
  assert.equal((await api(a,'/browser/session')).status,401);
  assert.equal((await cdp.call('Storage.getCookies',{browserContextId:context})).cookies.some(c=>c.name===old.name),false);
  console.log('PASS: real-browser HTTPS continuous OAuth/SIWE/rotation/consent/PKCE/MCP/logout checks.');
} catch {
  console.error('FAIL: browser validation did not complete. Credential-bearing diagnostics intentionally omitted.');process.exitCode=1;
} finally {
  if(cdp) {if(context)await cdp.call('Target.disposeBrowserContext',{browserContextId:context}).catch(()=>{});cdp.close();}
  for(const p of waiting.values()){clearTimeout(p.timer);p.reject(new Error('Fixture ended'));}signer.kill();lines.close();
}
