import {test} from 'node:test';import assert from 'node:assert/strict';import {readFileSync} from 'node:fs';import {validateBuild,validateRelease} from './preflight.mjs';
const file=name=>readFileSync(new URL(name,import.meta.url),'utf8');
test('release rejects mutable images and unapproved runtime',()=>{const e={RELEASE_RUNTIME_APPROVED:'yes',RELEASE_STORAGE_APPROVED:'yes',RELEASE_UID:'10001',RELEASE_GID:'10001'};for(const k of ['GO_IMAGE','FRONTEND_IMAGE','POSTGRES_IMAGE','NGINX_IMAGE'])e[k]='example/image@sha256:'+'a'.repeat(64);validateRelease(e);assert.throws(()=>validateRelease({...e,GO_IMAGE:'latest'}));assert.throws(()=>validateRelease({...e,RELEASE_UID:'0'}));assert.throws(()=>validateRelease({...e,RELEASE_RUNTIME_APPROVED:'no'}));});
test('release is private persistent and excludes financial/test activation',()=>{const c=file('compose.yaml');assert.match(c,/127\.0\.0\.1:8443:8443/);assert.match(c,/volumes: \[data:\/var\/lib\/postgresql\/data\]/);assert.match(c,/WIZPAY_SIWE_ENABLED: "false"/);assert.match(c,/DATABASE_MIGRATIONS_ON_START: "false"/);assert.match(c,/DATABASE_URL_FILE:/);assert.doesNotMatch(c,/privileged:|network_mode:|docker\.sock|browser-e2e/);});
test('runtime role cannot activate tenants or mutate financial records',()=>{const sql=file('runtime-role.sql');assert.doesNotMatch(sql,/GRANT (INSERT|UPDATE|DELETE) ON[^;]*\b(tenants|identities|wallet_bindings|intents|approvals)\b/);assert.match(sql,/REVOKE CREATE ON SCHEMA/);assert.doesNotMatch(sql,/PASSWORD/);});
test('runtime build context excludes secrets and synthetic signer',()=>{const ignore=file('Go.Dockerfile.dockerignore');assert.match(ignore,/^\*\*/);assert.doesNotMatch(ignore,/tls|signer|database_url|dbmigrate/);});

test('recovery invalidates restored authority without rewriting financial history',()=>{const s=file('restore-revoke.sql');assert.match(s,/current_database\(\) <> 'wizpay_mcp_restore'/);for(const table of ['browser_sessions','oauth_sessions','oauth_consents','oauth_access_tokens'])assert.ok(s.includes('UPDATE '+table+' SET revoked_at=COALESCE'));assert.doesNotMatch(s,/UPDATE (intents|approvals|siwe_challenges|siwe_authentications)/);});

test('release build bases must both be explicit immutable references', () => {
  const valid = {GO_BASE_IMAGE: 'example/go@sha256:' + 'a'.repeat(64), FRONTEND_BASE_IMAGE: 'example/node@sha256:' + 'b'.repeat(64)};
  validateBuild(valid);
  for (const key of Object.keys(valid)) {
    for (const value of [undefined, '', 'node:latest', 'node@sha256:abc', valid[key] + '\n']) {
      assert.throws(() => validateBuild({...valid, [key]: value}));
    }
  }
});

test('runtime requires separate durable storage approval', () => {
  const env = {RELEASE_RUNTIME_APPROVED: 'yes', RELEASE_UID: '10001', RELEASE_GID: '10001'};
  for (const key of ['GO_IMAGE', 'FRONTEND_IMAGE', 'POSTGRES_IMAGE', 'NGINX_IMAGE']) env[key] = 'example/image@sha256:' + 'a'.repeat(64);
  for (const value of [undefined, '', 'no']) assert.throws(() => validateRelease({...env, RELEASE_STORAGE_APPROVED: value}));
  validateRelease({...env, RELEASE_STORAGE_APPROVED: 'yes'});
});

test('release artifacts and frontend context remain isolated', () => {
  assert.equal(file('.gitignore').trim(), 'runtime/');
  for (const [name, arg] of [['Go', 'GO_BASE_IMAGE'], ['Frontend', 'FRONTEND_BASE_IMAGE']]) {
    const dockerfile = file(name + '.Dockerfile');
    assert.ok(dockerfile.includes('ARG ' + arg + '\nFROM ${' + arg + '}'));
    assert.doesNotMatch(dockerfile, /FROM .*:latest|deploy\/local|browser-e2e|signer/);
  }
  const frontend = file('Frontend.Dockerfile');
  assert.match(frontend, /USER node/);
  assert.match(frontend, /deploy\/release\/runtime\/frontend\/\.next/);
  const ignore = file('Frontend.Dockerfile.dockerignore');
  assert.match(ignore, /^\*\*/);
  assert.doesNotMatch(ignore, /tls|database|signer|!.*src|!.*\.env/);
  assert.match(ignore, /frontend\/\.next\/cache\//);
});
