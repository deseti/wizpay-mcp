import { pathToFileURL } from 'node:url';
export function validateBuild(env) {
  for (const key of ['GO_BASE_IMAGE', 'FRONTEND_BASE_IMAGE']) {
    if (!/^[a-zA-Z0-9./:_-]+@sha256:[0-9a-f]{64}(?![\s\S])/.test(env[key] ?? '')) throw new Error('Reviewed immutable build base required');
  }
}
export function validateRelease(env) {
  for(const key of ['GO_IMAGE','FRONTEND_IMAGE','POSTGRES_IMAGE','NGINX_IMAGE']) if(!/^[a-zA-Z0-9./:_-]+@sha256:[0-9a-f]{64}(?![\s\S])/.test(env[key]??'')) throw new Error('Reviewed immutable image digest required');
  if(!/^[1-9][0-9]*$/.test(env.RELEASE_UID??'')||!/^[1-9][0-9]*$/.test(env.RELEASE_GID??''))throw new Error('Nonroot TLS owner required');
  if(env.RELEASE_RUNTIME_APPROVED!=='yes')throw new Error('Private RC runtime approval required');
  if(env.RELEASE_STORAGE_APPROVED!=='yes')throw new Error('Durable host storage and off-host backup approval required');
}
if(process.argv[1]&&import.meta.url===pathToFileURL(process.argv[1]).href){
  if (process.argv.length === 3 && process.argv[2] === '--build') {
    validateBuild(process.env);
    console.log('Private RC build reference preflight PASS. No runtime or public-release approval.');
  } else if (process.argv.length === 2) {
    validateRelease(process.env);
    console.log('Private RC manifest preflight PASS. This is NOT public-release approval.');
  } else {
    throw new Error('Unsupported preflight arguments');
  }
}
