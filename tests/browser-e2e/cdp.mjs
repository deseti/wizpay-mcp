// Dependency-free CDP client. Connect only to an owner-started loopback browser.
export function validateBrowserArguments(args) {
  if (!Array.isArray(args) || !args.includes('--enable-automation') ||
      !args.includes('--remote-debugging-address=127.0.0.1') ||
      args.some(a => typeof a !== 'string' || /^--(?:no-sandbox|disable-setuid-sandbox|ignore-certificate-errors(?:-spki-list)?|allow-insecure-localhost|disable-web-security)(?:=|$)/.test(a))) {
    throw new Error('Isolated sandboxed TLS-verifying browser required');
  }
}
export class CDP {
  constructor(socket) {
    this.socket = socket; this.next = 0; this.pending = new Map(); this.listeners = new Set();
    socket.addEventListener('message', event => {
      const message = JSON.parse(String(event.data));
      if (message.id) {
        const p = this.pending.get(message.id); if (!p) return;
        this.pending.delete(message.id); clearTimeout(p.timer);
        message.error ? p.reject(new Error('Browser protocol operation failed')) : p.resolve(message.result);
      } else for (const listener of this.listeners) listener(message);
    });
    socket.addEventListener('close', () => this.fail());
    socket.addEventListener('error', () => this.fail());
  }
  fail() { for (const p of this.pending.values()) { clearTimeout(p.timer); p.reject(new Error('Browser disconnected')); } this.pending.clear(); }
  static async connect(raw) {
    const u = new URL(raw);
    if (u.protocol !== 'ws:' || !['127.0.0.1', '[::1]'].includes(u.hostname) || u.username || u.password || !u.pathname.startsWith('/devtools/browser/')) throw new Error('Owner-started loopback CDP endpoint required');
    const socket = new WebSocket(u);
    await new Promise((resolve, reject) => { const timer = setTimeout(() => { socket.close(); reject(new Error('Browser connection timed out')); }, 10000); socket.addEventListener('open', () => { clearTimeout(timer); resolve(); }, { once: true }); socket.addEventListener('error', () => { clearTimeout(timer); reject(new Error('Browser connection failed')); }, { once: true }); });
    return new CDP(socket);
  }
  call(method, params = {}, sessionId) {
    const id = ++this.next;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(id); reject(new Error('Browser operation timed out')); }, 20000);
      this.pending.set(id, { resolve, reject, timer });
      try { this.socket.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) })); } catch { clearTimeout(timer); this.pending.delete(id); reject(new Error('Browser disconnected')); }
    });
  }
  on(listener) { this.listeners.add(listener); return () => this.listeners.delete(listener); }
  close() { this.fail(); this.socket.close(); }
}
