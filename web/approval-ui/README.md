# WizPay Approval UI

This is the Web Approval UI foundation for the human-in-the-loop boundary. It is a Next.js App Router application using TypeScript and Tailwind CSS.

The browser talks only to the existing Approval HTTP API:

- `GET /approval/{approvalID}`
- `POST /approval/{approvalID}/decision`

The UI never handles wallet private keys, signing material, Circle tokens, JWTs, raw provider payloads, or transaction submission. Authentication is a placeholder boundary: requests use `credentials: "include"` so a future secure session cookie can be supplied by the hosting layer. No credential is written to browser storage.

## Local development

```bash
npm install
NEXT_PUBLIC_APPROVAL_API_BASE_URL=http://localhost:8080 npm run dev
```

Open `http://localhost:3000/approvals`.

The current Approval API exposes individual approval reads but no collection endpoint. Until one exists, configure the IDs shown on the list page with a comma-separated variable:

```bash
NEXT_PUBLIC_APPROVAL_IDS=apr_123,apr_456 npm run dev
```

Each ID is fetched through the centralized client in `src/lib/api.ts`.

## API configuration

`NEXT_PUBLIC_APPROVAL_API_BASE_URL` is optional. Leave it empty when the UI and API share an origin; set it to the API origin for local or separate-host development. The production placeholder is:

```text
https://mcp.wizpay.xyz
```

The API must permit the UI origin and secure credentialed requests when deployed cross-origin.

## Production boundary

The intended production UI domain is a future deployment such as `https://approval.wizpay.xyz`. Authentication/session exchange, CSRF protection, and origin policy must be supplied by the production edge/application boundary before enabling real user access. This frontend does not create approval authority, sign transactions, call Circle, or enable execution.
