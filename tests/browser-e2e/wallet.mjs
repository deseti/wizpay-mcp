// Injected only by the browser harness, never imported by the application.
export function walletScript(address) {
  return `(() => {
    if (location.origin !== 'https://connect.wizpay.xyz') return;
    const listeners = new Map(), pending = new Map(); let id = 0;
    const state = { address: ${JSON.stringify(address)}, chain: '0x13b2', rejectConnect: false, rejectSign: false, delay: false, calls: [] };
    const emit = (event, value) => { for (const fn of [...(listeners.get(event) || [])]) fn(value); };
    window.__wp3dWalletResult = (id, signature, error) => { const p = pending.get(id); if (!p) return; pending.delete(id); error ? p.reject({ code: 4001 }) : p.resolve(signature); };
    const provider = {
      async request({ method, params }) {
        state.calls.push(method);
        if (method === 'eth_requestAccounts') { if (state.rejectConnect) throw {code:4001}; return [state.address]; }
        if (method === 'eth_accounts') return [state.address];
        if (method === 'eth_chainId') return state.chain;
        if (method !== 'personal_sign') throw {code:4200};
        if (state.rejectSign) throw {code:4001};
        if (!Array.isArray(params) || params.length !== 2 || params[1] !== state.address || !/^0x(?:[0-9a-f]{2})+$/.test(params[0])) throw {code:-32602};
        const bytes = Uint8Array.from(params[0].slice(2).match(/../g), x => parseInt(x,16));
        const message = new TextDecoder('utf-8', {fatal:true}).decode(bytes);
        if (new TextEncoder().encode(message).some((x,i) => x !== bytes[i])) throw {code:-32602};
        const requestId = ++id;
        return new Promise((resolve,reject) => { pending.set(requestId,{resolve,reject});
          const send = () => window.__wp3dSign(JSON.stringify({id:requestId,message,address:state.address}));
          state.delay ? state.release = send : send();
        });
      },
      on(event,fn) { const list = listeners.get(event) || new Set(); list.add(fn); listeners.set(event,list); },
      removeListener(event,fn) { listeners.get(event)?.delete(fn); }
    };
    const announce = () => window.dispatchEvent(new CustomEvent('eip6963:announceProvider',{detail:{info:{uuid:'550e8400-e29b-41d4-a716-446655440000',name:'Synthetic Browser Wallet',rdns:'test.invalid'},provider}}));
    window.addEventListener('eip6963:requestProvider',announce);
    window.__wp3dWallet = { state, emit, announce };
  })();`;
}
