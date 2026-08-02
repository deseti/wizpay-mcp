# Approval UI boundary

The future React + Vite application is limited to login, wallet binding, transaction preview, explicit approval/rejection, policy management, and revocation. Phase 0 contains no frontend implementation, authentication, wallet connection, signing, or execution.

The UI must display a server-generated canonical intent summary and matching digest. It must never store signing secrets or claim that an MCP request, policy allow, provider response, or transaction hash is user approval or verified success.

