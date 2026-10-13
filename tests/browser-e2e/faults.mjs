// Response loss/corruption is injected only into this synthetic browser context.
// Never manufacture verification success. Held bodies/headers stay in memory.
export class Faults {
  constructor(cdp, session, policy = () => undefined, negativeResponse = () => {}) {
    this.cdp = cdp; this.session = session; this.policy = policy;
    this.counts = {session:0,verify:0,consent:0,logout:0};
    this.holds = []; this.tasks = new Set(); this.error = false;
    this.negativeResponse = negativeResponse;
  }
  async start() {
    this.detach = this.cdp.on(event => {
      if (event.sessionId !== this.session || event.method !== 'Fetch.requestPaused') return;
      const task = this.handle(event.params).catch(() => { this.error = true; });
      this.tasks.add(task); void task.finally(() => this.tasks.delete(task));
    });
    await this.cdp.call('Fetch.enable', {patterns:[
      {urlPattern:'https://connect.wizpay.xyz/browser/*',requestStage:'Request'},
      {urlPattern:'https://connect.wizpay.xyz/browser/session',requestStage:'Response'},
      {urlPattern:'https://connect.wizpay.xyz/browser/siwe/verify',requestStage:'Response'},
      {urlPattern:'https://connect.wizpay.xyz/browser/logout',requestStage:'Response'},
    ]}, this.session);
  }
  async handle(p) {
    const path = new URL(p.request.url).pathname;
    const response = p.responseStatusCode !== undefined || p.responseErrorReason !== undefined;
    const key = path === '/browser/session' ? 'session' : path === '/browser/siwe/verify' ? 'verify' : path === '/browser/consent' ? 'consent' : path === '/browser/logout' ? 'logout' : undefined;
    if (!response && key) this.counts[key]++;
    const action = this.policy({path,response,status:p.responseStatusCode,counts:this.counts});
    if (action === 'hold') { this.holds.push(p); return; }
    if (action === 'fail') { await this.cdp.call('Fetch.failRequest',{requestId:p.requestId,errorReason:'Aborted'},this.session); return; }
    if (typeof action === 'function') {
      if (new URL(p.request.url).origin !== 'https://connect.wizpay.xyz' || path !== '/browser/session' || p.responseStatusCode !== 200) throw new Error('Invalid negative fault');
      const body = await this.cdp.call('Fetch.getResponseBody',{requestId:p.requestId},this.session);
      const value = JSON.parse(body.base64Encoded ? Buffer.from(body.body,'base64').toString('utf8') : body.body);
      if (value.state !== 'AUTHENTICATED') throw new Error('Missing real authentication response');
      action(value); // Corrupt correlation only, then assert consent stays blocked.
      this.negativeResponse(p.networkId);
      await this.cdp.call('Fetch.fulfillRequest',{requestId:p.requestId,responseCode:200,
        responseHeaders:[{name:'Content-Type',value:'application/json'},{name:'Cache-Control',value:'no-store'}],
        body:Buffer.from(JSON.stringify(value)).toString('base64')},this.session);
      return;
    }
    await this.cdp.call(response ? 'Fetch.continueResponse' : 'Fetch.continueRequest',{requestId:p.requestId},this.session);
  }
  async held(path) {
    const deadline = Date.now()+20000;
    while (Date.now()<deadline) {
      this.check(); const p = this.holds.find(p => new URL(p.request.url).pathname === path);
      if (p) return p;
      await new Promise(resolve => setTimeout(resolve,25));
    }
    throw new Error('Browser fault checkpoint timed out');
  }
  async release(p) {
    this.holds = this.holds.filter(v => v !== p);
    await this.cdp.call(p.responseStatusCode === undefined ? 'Fetch.continueRequest' : 'Fetch.continueResponse',{requestId:p.requestId},this.session);
  }
  check() { if (this.error) throw new Error('Browser fault injection failed'); }
  async stop() {
    try {
      for (const p of [...this.holds]) { try { await this.release(p); } catch { /* Navigation may cancel the held fetch. */ } }
      await this.cdp.call('Fetch.disable',{},this.session);
      await Promise.all([...this.tasks]); this.check();
    } finally { this.detach?.(); }
  }
}
