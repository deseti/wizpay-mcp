import { resolve, join } from 'node:path';
import { realpath } from 'node:fs/promises';

export function environment(env, cwd) {
  if (env.WP3D_BROWSER_OWNER_APPROVED !== 'yes' || !/^[a-z0-9][a-z0-9-]{0,47}$/.test(env.PHASE_B_RUN_ID ?? '') || !/^ws:\/\/127\.0\.0\.1:9222\/devtools\/browser\/[A-Za-z0-9-]+$/.test(env.WP3D_BROWSER_CDP ?? '')) throw new Error('Approved isolated browser configuration required');
  const root = resolve(cwd,'tests/browser-e2e/runtime');
  const runtime = join(root,env.PHASE_B_RUN_ID);
  return {root,runtime,project:'wizpay-mcp-phaseb-'+env.PHASE_B_RUN_ID};
}
export async function verifyRuntime(config) {
  // Refuse symlinks outside the reserved test tree. Never create or overwrite
  // another run's files. The owner prepares this directory before execution.
  if (await realpath(config.root) !== config.root || await realpath(config.runtime) !== config.runtime) throw new Error('Isolated runtime path required');
}
export function diagnosticContains(value, expected) {
  return typeof value === 'string' && value.toLowerCase().includes(expected.toLowerCase());
}
