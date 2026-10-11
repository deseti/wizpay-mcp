// Offline Compose parsing only. Example identities are never started or pulled.
import {spawnSync} from 'node:child_process';
const env={...process.env,RELEASE_UID:'10001',RELEASE_GID:'10001',WIZPAY_ONBOARDING_TENANT_ID:'OWNER_APPROVED_TENANT'};
for(const key of ['GO_IMAGE','FRONTEND_IMAGE','POSTGRES_IMAGE','NGINX_IMAGE','OPERATOR_IMAGE'])env[key]='example/review@sha256:'+'a'.repeat(64);
const args=['compose','-f','deploy/release/compose.yaml','-f','deploy/release/postgres-tls.override.yaml','-f','deploy/release/siwe.override.yaml','config','--quiet'];
const result=spawnSync('docker',args,{env,encoding:'utf8',timeout:30000});
if(result.status!==0)throw new Error('Isolated Compose configuration validation failed');
console.log('PASS: isolated TLS/operator/SIWE Compose configuration; no services started');
