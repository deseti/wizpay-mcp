import { pathToFileURL } from 'node:url';
export function validateRelease(env) {
  for(const key of ['GO_IMAGE','FRONTEND_IMAGE','POSTGRES_IMAGE','NGINX_IMAGE']) if(!/^[a-zA-Z0-9./:_-]+@sha256:[0-9a-f]{64}$/.test(env[key]??'')) throw new Error('Reviewed immutable image digest required');
  if(!/^[1-9][0-9]*$/.test(env.RELEASE_UID??'')||!/^[1-9][0-9]*$/.test(env.RELEASE_GID??''))throw new Error('Nonroot TLS owner required');
  if(env.RELEASE_RUNTIME_APPROVED!=='yes')throw new Error('Private RC runtime approval required');
}
if(process.argv[1]&&import.meta.url===pathToFileURL(process.argv[1]).href){validateRelease(process.env);console.log('Private RC manifest preflight PASS. This is NOT public-release approval.');}
