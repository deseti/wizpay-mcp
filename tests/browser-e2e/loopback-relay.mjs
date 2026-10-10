// OPTIONAL OWNER-APPROVED mapping only. Does not terminate or bypass TLS.
import net from 'node:net';
if(process.env.WP3D_BROWSER_PORT_MAPPING_APPROVED!=='yes') throw new Error('Separate owner approval for loopback port 443 required');
const server=net.createServer(downstream=> {
  const upstream=net.connect({host:'127.0.0.1',port:9443});
  const close=()=>{downstream.destroy();upstream.destroy();};
  downstream.on('error',close);upstream.on('error',close);
  downstream.on('close',()=>upstream.destroy());upstream.on('close',()=>downstream.destroy());
  downstream.pipe(upstream).pipe(downstream);
});
server.on('error',()=>{console.error('Loopback relay could not bind; no fallback port or public binding used.');process.exitCode=1;});
server.listen(443,'127.0.0.1',()=>console.log('Owner-approved loopback-only TCP relay: 127.0.0.1:443 -> 127.0.0.1:9443. Ctrl-C removes it.'));
process.on('SIGINT',()=>{server.close();process.exit(0);});
