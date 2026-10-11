# Owner-controlled wallet enrollment

Migration 012 completes the pending-only foundation in migration 011. Production
bootstrap uses server-verified EOA enrollment followed by a separately delegated
owner application-identity decision. No legal-identity verification is claimed.
SIWE remains disabled by default; actual activation is an owner operation.

See [Claude Custom Connector implementation and activation](claude-custom-connector.md)
for the current policy, evidence model, isolated runtime/TLS setup, owner ceremony,
revocation, private operator commands and exact Claude configuration.

No real enrollment, public deployment or real Claude connection has been performed.
No administrative credentials or signing material enter the public application.
