import assert from 'node:assert/strict';

const idPattern = /^[0-9a-f]{64}$/;
export function validateFixture(f) {
  assert.ok(f && idPattern.test(f.fixture_id));
  const id = f.fixture_id;
  for (const [field, expected] of Object.entries({
    tenant_id: 'wp3d-browser-' + id, user_id: 'browser-user-' + id,
    other_user_id: 'other-browser-user-' + id, binding_id: 'binding-' + id,
    other_binding_id: 'other-binding-' + id, client_id: 'browser-client-' + id,
    owner_role: 'wp3d_owner_' + id.slice(0, 24),
  })) assert.equal(f[field], expected);
  for (const field of ['address', 'other_address']) assert.match(f[field], /^0x[0-9a-fA-F]{40}$/);
  assert.notEqual(f.address.toLowerCase(), f.other_address.toLowerCase());
}

// Owner-run SQL only. No proof insertion, direct activation, credential, or raw
// signature is generated. The production function validates immutable proof,
// tenant/user/binding/version, session_user delegation, freshness and replay.
export function activationSQL(f, proofs) {
  validateFixture(f);
  assert.equal(proofs.length, 2);
  assert.ok(proofs.every(p => typeof p === 'string' && idPattern.test(p)));
  assert.notEqual(proofs[0], proofs[1]);
  const calls = [
    [f.user_id, f.binding_id, proofs[0], 'primary'],
    [f.other_user_id, f.other_binding_id, proofs[1], 'secondary'],
  ].map(([user, binding, proof, label]) => `SELECT admin_activate_wallet('activate-${label}-${f.fixture_id}','${f.tenant_id}','${user}','${binding}','${proof}','synthetic-owner-approval',clock_timestamp()+interval '5 minutes');`).join('\n');
  return `\\set ON_ERROR_STOP on
BEGIN;
DO $$ BEGIN IF current_database() <> 'wizpay_mcp_browser_e2e' THEN RAISE EXCEPTION 'wrong test database'; END IF; END $$;
SET SESSION AUTHORIZATION ${f.owner_role};
${calls}
RESET SESSION AUTHORIZATION;
DO $$ BEGIN IF (SELECT count(*) FROM wallet_enrollment_approvals a JOIN wallet_enrollment_proofs p USING(challenge_id) JOIN wallet_bindings b ON b.tenant_id=a.tenant_id AND b.binding_id=a.binding_id WHERE a.tenant_id='${f.tenant_id}' AND b.status='ACTIVE' AND b.version=p.binding_version+1 AND b.verification_reference='enrollment:'||p.challenge_id) <> 2 THEN RAISE EXCEPTION 'synthetic activation evidence missing'; END IF; END $$;
COMMIT;
`;
}

export async function enrollWallets(h) {
  const proofs = [];
  for (const address of [h.fixture.address, h.fixture.other_address]) {
    const session = await h.pendingTab(h.fixture, address);
    const pending = (await h.api(session, '/browser/session')).body;
    await h.connect(session);
    await h.click(session, 'Request authentication message');
    await h.wait(session, () => !!document.querySelector('pre'));
    const c = await h.challengeFor(session);
    assert.equal(c.purpose, 'ENROLL');
    assert.match(c.message, /Owner approval is required/);
    assert.equal((await h.api(session, '/browser/consent', {decision:'grant'}, pending.csrf)).status, 403);
    // A valid-shaped invalid signature must not establish enrollment proof.
    assert.equal((await h.api(session, '/browser/siwe/verify', {challenge_id:c.challenge_id,signature:'0x'+'00'.repeat(65)}, pending.csrf)).status, 403);
    await h.click(session, 'Sign enrollment proof');
    await h.wait(session, () => /wallet control verified/i.test(document.querySelector('[role=status]')?.textContent ?? ''));
    await h.assertBlocked(session);
    // The UI logs out the still-pending enrollment session. No authentication or
    // consent is granted merely by signing; activation is a separate owner step.
    assert.equal((await h.api(session, '/browser/session')).status, 401);
    assert.equal((await h.api(session, '/browser/consent', {decision:'grant'}, pending.csrf)).status, 403);
    proofs.push(c.challenge_id);
  }
  return activationSQL(h.fixture, proofs);
}
