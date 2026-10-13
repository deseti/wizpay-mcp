import assert from 'node:assert/strict';
import { Faults } from './faults.mjs';

export const reliabilityStages = ['POST_VERIFY_RECOVERY','POST_VERIFY_READ_FAILURE','SESSION_CORRELATION_MISMATCH','SUPERSEDED_VERIFICATION','DISPOSED_AUTHENTICATION','DELAYED_LOGOUT','EXPIRED_AUTHORIZATION','EXPIRED_ACCESS_REAUTH'];
const pause = ms => new Promise(resolve => setTimeout(resolve,ms));

async function prepared(h) {
  const session = await h.pendingTab(h.fixture);
  const pending = (await h.api(session,'/browser/session')).body;
  await h.connect(session); await h.click(session,'Request authentication message');
  await h.wait(session,() => !!document.querySelector('pre'));
  await h.challengeFor(session);
  return {session,pending};
}
async function terminal(h, session) {
  await h.wait(session,() => /restart|replaced|does not match|expired/i.test(document.querySelector('[role=status]')?.textContent ?? ''));
  await h.assertBlocked(session);
}

export async function reliability(h) {
  for (const recover of [true,false]) {
    h.checkpoint(recover ? 'POST_VERIFY_RECOVERY' : 'POST_VERIFY_READ_FAILURE','01');
    const {session} = await prepared(h);
    const fault = new Faults(h.cdp,session,({path,response,counts}) => !response && path === '/browser/session' && (counts.session === 1 || !recover) ? 'fail' : undefined);
    await fault.start();
    try {
      await h.click(session,'Sign message to authenticate');
      if (recover) await h.wait(session,() => [...document.querySelectorAll('button')].some(b => b.textContent === 'Grant read-only access' && !b.disabled));
      else await terminal(h,session);
      await pause(300); fault.check();
      assert.equal(fault.counts.verify,1); assert.equal(fault.counts.session,2);
      assert.equal(fault.counts.consent,0); assert.equal(fault.counts.logout,0);
    } finally { await fault.stop(); }
    if (recover) assert.equal((await h.api(session,'/browser/session')).body.state,'AUTHENTICATED');
  }
  for (const field of ['transaction_id','challenge_id','csrf']) {
    h.checkpoint('SESSION_CORRELATION_MISMATCH','01');
    const {session,pending} = await prepared(h);
    const fault = new Faults(h.cdp,session,({path,response,status}) => path === '/browser/session' && response && status === 200 ? value => {
      value[field] = field === 'csrf' ? pending.csrf : field === 'challenge_id' ? '0'.repeat(64) : 'synthetic-other-transaction';
    } : undefined,h.negativeResponse);
    await fault.start();
    try {
      await h.click(session,'Sign message to authenticate'); await terminal(h,session); await pause(300); fault.check();
      assert.equal(fault.counts.verify,1); assert.equal(fault.counts.session,1);
      assert.equal(fault.counts.logout,0); assert.equal(fault.counts.consent,0);
    } finally { await fault.stop(); }
  }
  h.checkpoint('SUPERSEDED_VERIFICATION','01');
  {
    const {session:a} = await prepared(h);
    const fault = new Faults(h.cdp,a);
    await fault.start();
    try {
      const b = await h.pendingTab(h.fixture); const newer = (await h.api(b,'/browser/session')).body;
      await h.click(a,'Sign message to authenticate'); await terminal(h,a); await pause(300); fault.check();
      assert.equal(fault.counts.verify,1);
      assert.equal(fault.counts.logout,0); assert.equal(fault.counts.consent,0);
      assert.equal((await h.api(b,'/browser/session')).body.transaction_id,newer.transaction_id);
    } finally { await fault.stop(); }
  }
  h.checkpoint('DISPOSED_AUTHENTICATION','01');
  {
    const {session} = await prepared(h);
    const fault = new Faults(h.cdp,session,({path,response,status}) => response && status === 200 && path === '/browser/siwe/verify' ? 'hold' : undefined);
    await fault.start();
    try {
      await h.click(session,'Sign message to authenticate'); await fault.held('/browser/siwe/verify');
      await h.cdpCall('Page.navigate',{url:'about:blank'},session);
      await h.wait(session,() => location.href === 'about:blank');
      await pause(300);
      assert.equal(fault.counts.verify,1); assert.equal(fault.counts.session,0); assert.equal(fault.counts.logout,0); assert.equal(fault.counts.consent,0);
      // Browser navigation may cancel the response; do not claim a live old
      // controller was resumed. Exact dispose/late-resolution races are unit-tested.
    } finally { await fault.stop(); }
  }
  h.checkpoint('DELAYED_LOGOUT','01');
  {
    const a = await h.pendingTab(h.fixture), pa = (await h.api(a,'/browser/session')).body;
    await h.connect(a); await h.authenticate(a,pa);
    const fault = new Faults(h.cdp,a,({path,response,status}) => response && status === 200 && path === '/browser/logout' ? 'hold' : undefined);
    await fault.start();
    try {
      await h.evaluate(a,() => window.__wp3dWallet.emit('accountsChanged',[]));
      const held = await fault.held('/browser/logout');
      assert.equal((held.responseHeaders ?? []).some(header => header.name.toLowerCase() === 'set-cookie'),false);
      const b = await h.pendingTab(h.fixture), pb = (await h.api(b,'/browser/session')).body;
      await h.connect(b); const auth = await h.authenticate(b,pb);
      await fault.release(held); await terminal(h,a);
      const current = (await h.api(b,'/browser/session')).body;
      assert.equal(current.transaction_id,auth.transaction_id); assert.equal(current.challenge_id,auth.challenge_id); assert.equal(current.csrf,auth.csrf);
      assert.equal(fault.counts.logout,1); assert.equal(fault.counts.consent,0);
    } finally { await fault.stop(); }
  }
}

export async function expiryScenarios(h) {
  if (!h.longExpiry) { console.log('SKIP: real authorization/access-token expiry scenarios require WP3D_BROWSER_LONG_EXPIRY=yes.'); return; }
  h.checkpoint('EXPIRED_AUTHORIZATION','01');
  const session = await h.pendingTab(h.fixture), pending = (await h.api(session,'/browser/session')).body;
  // Wait for the actual five-minute transaction lifetime. Never alter clocks,
  // database timestamps, browser responses, or production token configuration.
  await pause(5*60*1000+2000);
  assert.equal((await h.api(session,'/browser/session')).status,401);
  assert.equal((await h.api(session,'/browser/siwe/challenge',{address:h.fixture.address},pending.csrf)).status,403);
  await h.cdpCall('Page.reload',{},session); await terminal(h,session);

  h.checkpoint('EXPIRED_ACCESS_REAUTH','01');
  const first = await h.authorizedToken();
  assert.ok(Number.isInteger(first.expires_in) && first.expires_in > 0 && first.expires_in <= 600);
  assert.equal((await h.mcp(first.access_token,'tools/list',{})).status,200);
  await pause(first.expires_in*1000+2000);
  const expired = await h.mcp(first.access_token,'tools/list',{});
  assert.equal(expired.status,401); assert.match(expired.challenge,/invalid_token/); assert.match(expired.challenge,/resource_metadata/);
  const fresh = await h.authorizedToken();
  assert.notEqual(fresh.access_token,first.access_token); assert.equal(fresh.scope,'mcp:read'); assert.equal(fresh.refresh_token,undefined);
  const listed = await h.mcp(fresh.access_token,'tools/list',{});
  assert.equal(listed.status,200);
  assert.deepEqual(listed.body.result.tools.map(t => t.name).sort(),['wizpay.get_approval','wizpay.get_intent']);
  const read = await h.mcp(fresh.access_token,'tools/call',{name:'wizpay.get_intent',arguments:{request_id:'phase-b-read',intent_id:'synthetic-missing-intent'}});
  assert.equal(read.status,200); // Domain not-found is permitted; transport must authorize the read.
  assert.equal(read.body.result.structuredContent.error.code,'intent_not_found');
  const approval = await h.mcp(fresh.access_token,'tools/call',{name:'wizpay.get_approval',arguments:{request_id:'phase-b-approval-read',approval_id:'synthetic-missing-approval'}});
  assert.equal(approval.status,200); assert.equal(approval.body.result.structuredContent.error.code,'approval_not_found');
  const denied = await h.mcp(fresh.access_token,'tools/call',{name:'wizpay.prepare_execution',arguments:{}});
  assert.equal(denied.body.error.code,-32602);
}
