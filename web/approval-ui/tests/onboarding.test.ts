import { test } from "node:test";
import assert from "node:assert/strict";
import { discover, isProvider, type Provider } from "../src/lib/wallet/provider";
import { accounts, chain, messageHex, sign } from "../src/lib/wallet/signing";
import { browserAPI, challenge, redirect, view, type API, type View } from "../src/lib/onboarding-api";
import { Flow } from "../src/lib/onboarding-flow";
const account = "0x" + "a".repeat(40), other = "0x" + "b".repeat(40);
const signature = "0x" + "1".repeat(130);
const pending: View = { state: "PENDING", transaction_id: "transaction-A", client_id: "client", client_name: "Client", resource: "https://mcp.wizpay.xyz/mcp", scope: "mcp:read", csrf: "a".repeat(43), authentication_available: true };
const authenticated: View = { ...pending, state: "AUTHENTICATED", challenge_id: "a".repeat(64), user_id: "user", csrf: "b".repeat(43) };
const c = { challenge_id: "a".repeat(64), message: "Exact\nmessage é🙂\n", address: account, expires_at: new Date(Date.now()+60000).toISOString() };
function deferred<T>() { let resolve!: (v: T) => void; const promise = new Promise<T>(r => { resolve=r; }); return { promise, resolve }; }
class Mock implements Provider {
  list = [account]; network = "0x13b2"; calls: {method:string;params?:readonly unknown[]}[]=[];
  listeners = new Map<string, Set<(...a: unknown[]) => void>>();
  hook?: (method: string) => Promise<unknown>;
  async request(a: {method:string;params?:readonly unknown[]}) { this.calls.push(a); if(this.hook) return this.hook(a.method); return a.method === "eth_chainId" ? this.network : a.method === "personal_sign" ? signature : this.list; }
  on(e:string,l:(...a:unknown[])=>void){ if(!this.listeners.has(e))this.listeners.set(e,new Set());this.listeners.get(e)!.add(l); }
  removeListener(e:string,l:(...a:unknown[])=>void){this.listeners.get(e)?.delete(l);}
  emit(e:string){for(const l of this.listeners.get(e)??[])l();}
}
function fixture() {
  const p=new Mock(); let current=pending; const calls:string[]=[];
  const api:API={session:async()=>current,challenge:async()=>c,verify:async()=>{calls.push("verify");current=authenticated;},consent:async(csrf,d)=>{calls.push(d+":"+csrf);return d==="grant"?"https://client.example/callback?code=x":undefined;},logout:async csrf=>{calls.push("logout:"+csrf);current=pending;}};
  const f=new Flow(api,()=>{},u=>calls.push("navigate:"+u));
  return {p,api,f,calls,set:(v:View)=>{current=v;}};
}
async function ready(x:ReturnType<typeof fixture>){await x.f.initialize();x.f.select({id:"wallet",name:"Wallet",provider:x.p});await x.f.connect();await x.f.requestChallenge();}
test("discovery: missing, shape, multiple, duplicate, legacy and cleanup",()=>{
 const target=new EventTarget() as EventTarget & {ethereum?:unknown};let result:unknown[]=[];
 let cleanup=discover(target,v=>result=v);assert.equal(result.length,0);cleanup();assert.equal(isProvider({request(){}}),false);
 target.ethereum=new Mock();cleanup=discover(target,v=>result=v);assert.equal(result.length,1);
 const announce=(p:Provider,id:string)=>{const event=new Event("eip6963:announceProvider");Object.defineProperty(event,"detail",{value:{provider:p,info:{uuid:id,name:"<svg>untrusted</svg>"}}});target.dispatchEvent(event);};
 const p=new Mock();announce(p,"12345678-1234-4123-8123-123456789012");announce(p,"12345678-1234-4123-8123-123456789012");assert.equal(result.length,2);
 announce(new Mock(),"12345678-1234-4123-8123-123456789013");assert.equal(result.length,3);cleanup();announce(new Mock(),"12345678-1234-4123-8123-123456789014");assert.equal(result.length,3);
});
test("exact UTF8 and personal_sign only, account/network checks",async()=>{
 const p=new Mock();await sign(p,account,c.message);assert.deepEqual(p.calls.find(a=>a.method==="personal_sign")?.params,["0x"+Buffer.from(c.message,"utf8").toString("hex"),account]);assert.equal(messageHex(c.message),"0x"+Buffer.from(c.message).toString("hex"));
 p.list=[other];await assert.rejects(sign(p,account,c.message));p.list=[account];p.network="0x1";await assert.rejects(sign(p,account,c.message));assert.equal(chain("0x13b2"),true);assert.throws(()=>accounts(["invalid"]));
 assert.ok(p.calls.every(a=>["eth_accounts","eth_chainId","personal_sign"].includes(a.method)));
});
test("explicit selection, multiple accounts, network correction and permission rejection",async()=>{
 const x=fixture();await x.f.initialize();await x.f.connect();assert.equal(x.p.calls.length,0);x.f.select({id:"x",name:"x",provider:x.p});x.p.list=[account,other];x.p.network="0x1";await x.f.connect();assert.equal(x.f.state.account,undefined);assert.equal(x.f.state.phase,"network");x.p.network="0x13b2";await x.f.connect();x.f.chooseAccount(other);assert.equal(x.f.state.account,other);
 x.p.hook=async()=>{throw {code:4001};};await x.f.connect();assert.equal(x.f.state.phase,"rejected");
});
test("authentication rotates CSRF; grant is a separate explicit action",async()=>{
 const x=fixture();await ready(x);assert.equal(x.f.canGrant,false);await x.f.decide("grant");assert.equal(x.calls.length,0);await x.f.requestChallenge();await x.f.authenticate();assert.equal(x.f.canGrant,true);assert.deepEqual(x.calls,["verify"]);await x.f.decide("grant");assert.ok(x.calls.includes("grant:"+authenticated.csrf));assert.ok(x.calls.some(v=>v.startsWith("navigate:")));
});
test("denial uses local behavior, no callback",async()=>{const x=fixture();await x.f.initialize();await x.f.decide("deny");assert.equal(x.f.state.phase,"denied");assert.equal(x.calls.some(v=>v.startsWith("navigate:")),false);});
test("wrong challenge, expired session and verification rejection fail closed",async()=>{
 const x=fixture();await ready(x);x.api.challenge=async()=>({...c,address:other});await x.f.requestChallenge();assert.equal(x.f.state.challenge,undefined);
 x.api.session=async()=>{throw new Error("expired");};await x.f.initialize();assert.equal(x.f.canGrant,false);
 const y=fixture();await ready(y);y.api.verify=async()=>{throw new Error("rejected");};await y.f.authenticate();assert.equal(y.f.canGrant,false);assert.equal(y.f.state.restart,true);
});
for(const event of ["accountsChanged","chainChanged","disconnect"]) test(event+" during signing discards late signature",async()=>{
 const x=fixture();await ready(x);const d=deferred<unknown>();x.p.hook=async m=>m==="personal_sign"?d.promise:m==="eth_chainId"?"0x13b2":[account];const work=x.f.authenticate();await new Promise<void>(r=>setImmediate(r));x.p.emit(event);d.resolve(signature);await work;assert.equal(x.calls.includes("verify"),false);assert.equal(x.f.canGrant,false);assert.equal(x.f.state.restart,true);
});
test("provider change while verification commits reconciles fresh cookie/CSRF and logs out",async()=>{
 const x=fixture();await ready(x);const d=deferred<void>();x.api.verify=async()=>{await d.promise;x.set(authenticated);};const work=x.f.authenticate();await new Promise<void>(r=>setImmediate(r));x.f.select({id:"other",name:"Other",provider:new Mock()});assert.equal(x.f.canGrant,false);d.resolve();await work;assert.ok(x.calls.includes("logout:"+authenticated.csrf));assert.equal(x.f.state.restart,true);
});
test("lost verification response and failed session refresh never enable consent",async()=>{
 const x=fixture();await ready(x);x.api.verify=async()=>{x.set(authenticated);throw new Error("response_lost");};await x.f.authenticate();assert.ok(x.calls.includes("logout:"+authenticated.csrf));assert.equal(x.f.canGrant,false);
 const y=fixture();await ready(y);y.api.session=async()=>{throw new Error("unavailable");};await y.f.authenticate();assert.equal(y.f.canGrant,false);assert.equal(y.f.state.restart,true);
});
test("concurrent authentication cannot submit twice; lost grant cannot be replayed",async()=>{
 const x=fixture();await ready(x);await Promise.all([x.f.authenticate(),x.f.authenticate()]);assert.equal(x.calls.filter(c=>c==="verify").length,1);let grants=0;x.api.consent=async()=>{grants++;throw new Error("lost");};await x.f.decide("grant");await x.f.decide("grant");assert.equal(grants,1);
});
test("safe response validation and redirects",()=>{
 for(const url of ["http://client.example","javascript:alert(1)","https://user:secret@client.example","https://client.example/#secret"])assert.throws(()=>redirect(url));
 assert.equal(redirect("https://client.example/callback"),"https://client.example/callback");assert.throws(()=>view({...pending,scope:"write"}));assert.throws(()=>challenge({...c,challenge_id:"bad"}));
});
test("API uses same-origin cookies, fresh CSRF and empty logout; errors reject",async()=>{
 const original=globalThis.fetch;const requests:RequestInit[]=[];
 globalThis.fetch=async(_input,init)=>{requests.push(init!);return new Response(JSON.stringify({state:"complete"}));};
 try{await browserAPI().logout(authenticated.csrf);assert.equal(requests[0].credentials,"same-origin");assert.equal(requests[0].body,undefined);assert.equal((requests[0].headers as Record<string,string>)["X-CSRF-Token"],authenticated.csrf);
 globalThis.fetch=async()=>new Response("{}",{status:403});await assert.rejects(browserAPI().challenge(pending.csrf,account));}finally{globalThis.fetch=original;}
});
test("initial permission exposure is checked without treating it as authentication",async()=>{
 const x=fixture();await x.f.initialize();x.f.select({id:"x",name:"x",provider:x.p});x.p.hook=async m=>{if(m==="eth_requestAccounts")x.p.emit("accountsChanged");return m==="eth_chainId"?"0x13b2":[account];};await x.f.connect();assert.equal(x.f.state.phase,"connected");assert.equal(x.f.canGrant,false);
});
test("signature rejection and challenge expiry cannot authenticate",async()=>{
 const x=fixture();await ready(x);x.p.hook=async m=>{if(m==="personal_sign")throw {code:4001};return m==="eth_chainId"?"0x13b2":[account];};await x.f.authenticate();assert.equal(x.f.state.phase,"rejected");assert.equal(x.calls.includes("verify"),false);
 const y=fixture();await ready(y);y.api.challenge=async()=>({...c,expires_at:"2000-01-01T00:00:00Z"});await y.f.requestChallenge();assert.equal(y.f.state.challenge,undefined);
});
test("stale CSRF refresh and cross-client refresh require guarded cleanup",async()=>{
 for(const next of [{...authenticated,csrf:pending.csrf},{...authenticated,client_id:"other"}]){const x=fixture();await ready(x);x.api.verify=async()=>x.set(next);await x.f.authenticate();assert.equal(x.f.canGrant,false);assert.ok(x.calls.some(c=>c.startsWith("logout:")));}
});
test("late consent after wallet change never navigates",async()=>{
 const x=fixture();await ready(x);await x.f.authenticate();const d=deferred<string>();x.api.consent=async()=>d.promise;const work=x.f.decide("grant");await new Promise<void>(r=>setImmediate(r));x.p.emit("accountsChanged");d.resolve("https://client.example/callback?code=x");await work;assert.equal(x.calls.some(c=>c.startsWith("navigate:")),false);assert.equal(x.f.canGrant,false);
});
for (const method of ["eth_accounts","eth_chainId"]) for (const event of ["accountsChanged","chainChanged","provider","dispose"]) test(`pre-sign ${method}: ${event} never signs`,async()=>{
 const x=fixture();await ready(x);const d=deferred<unknown>();let blocked=false;
 x.p.hook=async m=>{if(m===method&&!blocked){blocked=true;return d.promise;}return m==="eth_chainId"?"0x13b2":[account];};
 const work=x.f.authenticate();await new Promise<void>(r=>setImmediate(r));
 if(event==="dispose")x.f.dispose();else if(event==="provider")x.f.select({id:"new",name:"new",provider:new Mock()});else x.p.emit(event);
 d.resolve(method==="eth_chainId"?"0x13b2":[account]);await work;
 assert.equal(x.p.calls.filter(c=>c.method==="personal_sign").length,0);assert.equal(x.calls.includes("verify"),false);
});
for(const substitute of [{...authenticated,challenge_id:"b".repeat(64),user_id:"other"},{...authenticated,transaction_id:"transaction-B"},{...authenticated,challenge_id:undefined}]) test("substituted or missing evidence never grants or logs out another flow",async()=>{
 const x=fixture();await ready(x);x.api.verify=async()=>x.set(substitute);await x.f.authenticate();assert.equal(x.f.canGrant,false);assert.equal(x.f.state.restart,true);assert.equal(x.calls.some(c=>c.startsWith("logout:")),false);
});
test("provider exceptions and malformed rejections remain contained",async()=>{
 assert.equal(isProvider(Object.defineProperty({},"request",{get(){throw new Error("getter");}})),false);
 const t=new EventTarget() as EventTarget & {ethereum?:unknown};Object.defineProperty(t,"ethereum",{get(){throw new Error("getter");}});const stop=discover(t,()=>{});stop();
 const x=fixture();await x.f.initialize();const p=new Mock();p.on=()=>{throw new Error("registration");};p.removeListener=()=>{throw new Error("cleanup");};assert.doesNotThrow(()=>x.f.select({id:"bad",name:"bad",provider:p}));assert.doesNotThrow(()=>x.f.dispose());
 const y=fixture();await ready(y);y.p.hook=async()=>{throw null;};await y.f.authenticate();assert.equal(y.f.canGrant,false);
});
test("failed removal leaves old provider listeners inert after replacement",async()=>{
 const x=fixture();await ready(x);const old=x.p;old.removeListener=()=>{throw new Error("cannot_remove");};
 const next=new Mock();x.f.select({id:"next",name:"Next",provider:next});old.emit("disconnect");
 assert.equal(x.f.state.restart,false);assert.equal(x.f.state.wallet?.provider,next);await x.f.connect();assert.equal(x.f.state.phase,"connected");
 x.f.dispose();next.emit("disconnect");assert.equal(x.f.state.restart,false);
});
test("partial event registration failure clears provider and contains leaked listener",async()=>{
 const x=fixture();await x.f.initialize();const bad=new Mock();const attach=bad.on.bind(bad);let count=0;
 bad.on=(event,listener)=>{attach(event,listener);if(++count===2)throw new Error("registration");};bad.removeListener=()=>{throw new Error("cleanup");};
 x.f.select({id:"bad",name:"Bad",provider:bad});assert.equal(x.f.state.wallet,undefined);bad.emit("accountsChanged");assert.equal(x.f.state.restart,false);assert.equal(x.f.canGrant,false);
});
test("throwing rejection getters and malformed metadata cannot restore authority",async()=>{
 const x=fixture();await ready(x);x.p.hook=async()=>{throw Object.defineProperty({},"code",{get(){throw new Error("reject_getter");}});};await x.f.authenticate();assert.equal(x.f.canGrant,false);assert.equal(x.f.state.busy,false);
 const target=new EventTarget();let count=0;const stop=discover(target,v=>{count=v.length;});const e=new Event("eip6963:announceProvider");Object.defineProperty(e,"detail",{get(){throw new Error("metadata_getter");}});assert.doesNotThrow(()=>target.dispatchEvent(e));assert.equal(count,0);stop();
});
test("malformed rejection prototype is contained",async()=>{
 const x=fixture();await ready(x);x.p.hook=async()=>{throw new Proxy({}, {getPrototypeOf(){throw new Error("prototype");}});};
 await assert.doesNotReject(x.f.authenticate());assert.equal(x.f.canGrant,false);assert.equal(x.f.state.busy,false);
});
