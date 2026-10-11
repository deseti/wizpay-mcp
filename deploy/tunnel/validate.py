#!/usr/bin/env python3
"""Offline, disposable Nginx TLS/HTTP checks. No browser or public Tunnel starts."""
import os
from pathlib import Path
import subprocess
import tempfile
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]

def run(*args, **kwargs):
    return subprocess.run(args, check=True, text=True, capture_output=True, timeout=30, **kwargs).stdout.strip()

image = run('docker', 'image', 'inspect', '--format', '{{.Id}}', 'nginx:stable-alpine')
name = 'wizpay-mcp-ingress-test-' + uuid.uuid4().hex[:12]
with tempfile.TemporaryDirectory(prefix='wizpay-mcp-ingress-') as directory:
    work = Path(directory)
    (work / 'runtime').mkdir()
    run('openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
        '-subj', '/CN=mcp.wizpay.xyz', '-addext', 'subjectAltName=DNS:mcp.wizpay.xyz,DNS:connect.wizpay.xyz',
        '-keyout', str(work/'runtime/tls.key'), '-out', str(work/'runtime/tls.crt'))
    os.chmod(work/'runtime/tls.key', 0o600)
    config = (ROOT/'deploy/tunnel/nginx.conf').read_text()
    # Mock upstream lives in this disposable container. Real backend behavior is
    # independently exercised by PostgreSQL-backed Go protocol tests.
    config = config.replace('http {', 'http {\n server { listen 3000; listen 8080; location / { return 200 "$http_host|$http_x_forwarded_proto|$http_forwarded|$http_cf_connecting_ip"; } }', 1)
    (work/'nginx.conf').write_text(config)
    (work/'proxy-common.conf').write_text((ROOT/'deploy/local/proxy-common.conf').read_text())
    common = ['--network', 'none', '--add-host', 'go:127.0.0.1', '--add-host', 'frontend:127.0.0.1',
              '--mount', f'type=bind,src={work},dst=/work,readonly', '--read-only', '--tmpfs', '/tmp',
              '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges:true', '--user', f'{os.getuid()}:{os.getgid()}',
              '--entrypoint', 'nginx']
    run('docker', 'run', '--rm', *common, image, '-p', '/work/', '-c', 'nginx.conf', '-t')
    print('PASS: nginx -t (cached immutable image, no network)')
    # Bridge network permits loopback-only HTTP testing; no public listeners.
    common[1] = 'bridge'
    try:
        run('docker', 'run', '-d', '--name', name, *common, '-p', '127.0.0.1::8443', image,
            '-p', '/work/', '-c', 'nginx.conf', '-g', 'daemon off;')
        port = run('docker', 'port', name, '8443/tcp').rsplit(':',1)[1]
        def request(host, path, headers=(), expected=200, response=None):
            args=['curl','--silent','--show-error','--noproxy','*','--max-time','3',
                  '--cacert',str(work/'runtime/tls.crt'),'--connect-to',f'{host}:443:127.0.0.1:{port}',
                  '-H','X-Forwarded-Proto: https','-H','CF-Connecting-IP: 192.0.2.1','-H','X-Forwarded-For: 192.0.2.1']
            for header in headers: args+=['-H',header]
            result=run(*args,'-w','\n%{http_code}',f'https://{host}{path}')
            body,status=result.rsplit('\n',1)
            assert int(status)==expected,(host,path,status,expected)
            if response is not None: assert body==response,(body,response)
        for attempt in range(20):
            try:
                request('mcp.wizpay.xyz','/mcp',response='mcp.wizpay.xyz|||')
                break
            except subprocess.CalledProcessError:
                if attempt==19: raise
                time.sleep(0.1)
        for host,path in [('connect.wizpay.xyz','/onboarding'),('connect.wizpay.xyz','/oauth/authorize'),
                          ('connect.wizpay.xyz','/.well-known/oauth-authorization-server'),
                          ('mcp.wizpay.xyz','/.well-known/oauth-protected-resource/mcp')]:
            request(host,path,response=host+'|||')
        for path in ['/health','/readiness','/debug','/approval/id','/oauth/register','/browser/admin']:
            for host in ['mcp.wizpay.xyz','connect.wizpay.xyz']: request(host,path,expected=404)
        for header in ['Forwarded: proto=https','X-Forwarded-Host: attacker.example','X-Forwarded-Proto: http',
                       'X-Forwarded-For: 192.0.2.2','X-Forwarded-Port: 443','X-Real-IP: 192.0.2.1',
                       'X-Forwarded-Server: attacker','X-Original-Host: attacker']:
            request('mcp.wizpay.xyz','/mcp',[header],expected=400)
        request('mcp.wizpay.xyz','/mcp',['Host: connect.wizpay.xyz'],expected=421)
        request('mcp.wizpay.xyz','/mcp',['Host: attacker.example'],expected=421)
        print('PASS: canonical Host/SNI, TLS, route denial, proxy spoof rejection and upstream header stripping')
    finally:
        subprocess.run(['docker','rm','-f',name],capture_output=True,timeout=30)
