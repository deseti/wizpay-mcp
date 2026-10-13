import { request } from 'node:https';

export const testPort = 9443;
export const resolverRules = 'MAP connect.wizpay.xyz:443 127.0.0.1:9443, MAP mcp.wizpay.xyz:443 127.0.0.1:9443, MAP * ~NOTFOUND';
const paths = new Map([
  ['connect.wizpay.xyz', new Set(['/oauth/token','/.well-known/oauth-authorization-server'])],
  ['mcp.wizpay.xyz', new Set(['/.well-known/oauth-protected-resource/mcp'])],
]);

export function optionsFor(raw, ca, body) {
  const url = new URL(raw);
  if (url.protocol !== 'https:' || url.port || url.username || url.password || url.search || url.hash || !paths.get(url.hostname)?.has(url.pathname)) throw new Error('Local test transport rejected');
  return {
    hostname:'127.0.0.1', port:testPort, path:url.pathname, agent:false,
    servername:url.hostname, ca, rejectUnauthorized:true,
    method:body === undefined ? 'GET' : 'POST',
    headers:{Host:url.hostname, ...(body === undefined ? {} : {'Content-Type':'application/x-www-form-urlencoded','Content-Length':Buffer.byteLength(body)})},
  };
}

export async function localRequest(url, ca, body) {
  const options = optionsFor(url, ca, body);
  return new Promise((resolve, reject) => {
    const req = request(options, async res => {
      try {
        if (res.socket.remoteAddress !== '127.0.0.1' || res.socket.remotePort !== testPort || !res.socket.authorized) throw new Error();
        const chunks = []; let bytes = 0;
        for await (const chunk of res) { bytes += chunk.length; if (bytes > 65536) throw new Error(); chunks.push(chunk); }
        resolve({status:res.statusCode,body:JSON.parse(Buffer.concat(chunks).toString('utf8'))});
      } catch { reject(new Error('Local HTTPS response failed')); }
    });
    req.on('error', () => reject(new Error('Local HTTPS request failed')));
    req.setTimeout(20000, () => req.destroy());
    req.end(body);
  });
}
