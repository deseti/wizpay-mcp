import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const root = new URL('./', import.meta.url);
const read = name => readFileSync(new URL(name, root), 'utf8');
const edge=read('nginx.conf'), common=read('proxy-common.conf'), db=read('compose.yaml');
test('container edge rejects unknown authority and publishes loopback only',()=>{
 const listeners=[...edge.matchAll(/listen ([^;]+);/g)].map(m=>m[1]);
 assert.equal(listeners.length,3); assert.ok(listeners.every(v=>v.startsWith('8443 ssl')));
 assert.match(edge,/default_server/);assert.match(edge,/return 421/);
 for(const host of ['connect.wizpay.xyz','mcp.wizpay.xyz']) assert.ok(edge.includes(`if ($http_host != "${host}")`));
});
test('routing allowlist separates browser and MCP without approval routes',()=>{
 for(const route of ['/onboarding','/_next/','/browser/','/oauth/','/.well-known/oauth-authorization-server','/mcp','/.well-known/oauth-protected-resource/mcp']) assert.ok(edge.includes(route));
 assert.equal((edge.match(/location \/ \{ return 404;/g)||[]).length,2);
 assert.ok(!edge.includes('/approval'));
});
test('forwarding uses an explicit header allowlist preserving protocol authority',()=>{
 assert.match(common,/proxy_pass_request_headers off;/);
 for(const [key,value] of [['Host','$host'],['Origin','$http_origin'],['X-CSRF-Token','$http_x_csrf_token'],['Authorization','$http_authorization'],['Cookie','$http_cookie'],['MCP-Protocol-Version','$http_mcp_protocol_version']]) assert.ok(common.includes(`proxy_set_header ${key} ${value};`));
 for(const line of common.split('\n').filter(l=>l.startsWith('proxy_set_header'))) assert.ok(!/Forwarded/i.test(line));
});
test('no logging credentials, cookie rewriting, cache, redirect rewriting or CORS',()=>{
 assert.match(edge,/access_log off;/);assert.match(edge,/error_log stderr crit;/);
 assert.match(common,/proxy_cache off;/);assert.match(common,/proxy_redirect off;/);
 assert.ok(!/proxy_cookie|Access-Control-Allow-Origin|log_format/.test(edge+common));
 assert.match(edge,/Cache-Control "no-store" always/);
});
test('body limits and MCP-compatible upstream timeouts are route-specific',()=>{
 assert.match(edge,/client_body_timeout 5s/);assert.match(edge,/client_max_body_size 1k/);assert.match(edge,/client_max_body_size 8k/);
 assert.match(common,/proxy_read_timeout 300s/);assert.match(common,/proxy_buffering off/);assert.match(common,/proxy_request_buffering off/);
});
test('disposable PostgreSQL never pulls, exposes loopback only and has no persistent volume',()=>{
 assert.match(db,/pull_policy: never/);assert.match(db,/127.0.0.1:8443:8443/);assert.equal((db.match(/ports:/g)||[]).length,1);assert.match(db,/tmpfs:/);assert.ok(!/container_name:|external:|network_mode:|privileged:|docker.sock/.test(db));assert.match(db,/internal: true/);assert.match(db,/wizpay-mcp-wp3d-local/);
});
test('local defaults preserve disabled execution and authentication activation',()=>{
 const env=read('app.env.example');for(const entry of ['SERVER_HOST=\n','AUTH_REQUIRED=true','OAUTH_ENABLED=true','WIZPAY_SIWE_ENABLED=false','WIZPAY_AUTONOMY_ENABLED=false'])assert.ok(env.includes(entry));
});

test('private DNS upstreams and read-only TLS mounts',()=>{
 assert.ok(!edge.includes('http://127.0.0.1'));assert.match(edge,/http:\/\/go:8080/);assert.match(edge,/http:\/\/frontend:3000/);
 assert.match(db,/tls.key:ro/);assert.match(db,/tls.crt:ro/);
 assert.equal((db.match(/networks: \[private(?:, edge)?\]/g)||[]).length,4);
 for(const header of ['forwarded','x_forwarded_host','x_forwarded_proto','x_forwarded_for'])assert.ok(edge.includes(`if ($http_${header} != "") { return 400; }`));
});
test('artifact builds are offline and omit credentials from contexts',()=>{
 assert.equal((db.match(/network: none/g)||[]).length,2);
 for (const file of ['Go.Dockerfile','Frontend.Dockerfile']) {
  assert.ok(!/^RUN /m.test(read(file)));
  assert.match(read(file+'.dockerignore'),/^\*\*/);
  assert.ok(!read(file+'.dockerignore').includes('!tls'));
 }
 assert.match(db,/WP3D_LOCAL_UID/);assert.match(db,/no-new-privileges:true/);
});
test('Go build context admits only its runtime binary, not TLS or environment files',()=>{
 const rules=read('Go.Dockerfile.dockerignore').trim().split('\n');
 assert.deepEqual(rules,['**','!deploy/','!deploy/local/','!deploy/local/runtime/','!deploy/local/runtime/wizpay-mcp-server']);
});
test('all Nginx temporary paths and PID remain on writable tmpfs',()=>{
 for (const directive of ['client_body_temp_path','proxy_temp_path','fastcgi_temp_path','uwsgi_temp_path','scgi_temp_path']) {
  const matches=[...edge.matchAll(new RegExp(`\\b${directive}\\s+([^;]+);`, 'g'))];
  assert.equal(matches.length,1,directive);
  assert.match(matches[0][1],/^\/tmp\/[a-z]+$/);
 }
 assert.match(edge,/pid \/tmp\/nginx\.pid;/);
 const nginx=db.slice(db.indexOf('  nginx:'),db.indexOf('  go:'));
 assert.match(nginx,/tmpfs: \[\/tmp\]/);
 assert.match(nginx,/read_only: true/);
 assert.match(nginx,/user:.*WP3D_LOCAL_UID.*WP3D_LOCAL_GID/);
 assert.match(nginx,/cap_drop: \[ALL\]/);
 assert.match(nginx,/no-new-privileges:true/);
});

test('edge network belongs only to nginx with one loopback publication',()=>{
 const services=[...db.matchAll(/^  (nginx|go|frontend|postgres):\n([\s\S]*?)(?=^  [a-z]+:|^networks:)/gm)];
 assert.equal(services.length,4);
 for(const [,name,body] of services){
  assert.match(body,name==='nginx'?/networks: \[private, edge\]/:/networks: \[private\]/);
  if(name==='nginx')assert.match(body,/ports:\n      - "127\.0\.0\.1:8443:8443"/);
  else assert.ok(!/\bports:/.test(body));
 }
 assert.equal((db.match(/ports:/g)||[]).length,1);
 assert.equal((db.match(/127\.0\.0\.1:8443:8443/g)||[]).length,1);
 assert.match(db,/private:\n    internal: true/);
 assert.match(db,/edge:\n    driver: bridge/);
 assert.ok(!/network_mode:|privileged:|external:/.test(db));
});
