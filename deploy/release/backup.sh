#!/usr/bin/env bash
# OWNER-RUN ONLY. Encrypted stream; no plaintext database dump on disk.
set -euo pipefail
test "${RELEASE_BACKUP_APPROVED:-}" = yes
test -n "${BACKUP_RECIPIENT:-}"
command -v gpg >/dev/null || { echo 'Approved encryption tooling unavailable'; exit 1; }
umask 077
mkdir -p deploy/release/runtime/backups
target="deploy/release/runtime/backups/$(date -u +%Y%m%dT%H%M%SZ).dump.gpg"
test ! -e "$target"
trap 'rm -f "$target.part"' EXIT
docker compose -p wizpay-mcp-rc -f deploy/release/compose.yaml exec -T postgres \
  pg_dump -U wizpay_migrator -d wizpay_mcp -Fc | \
  gpg --batch --encrypt --recipient "$BACKUP_RECIPIENT" --output "$target.part"
mv "$target.part" "$target"
sha256sum "$target" > "$target.sha256"
echo 'Encrypted backup created. Off-host copy and restore verification remain operator requirements.'
