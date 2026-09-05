# Kosmos

Kosmos is an MIT-licensed, extensible small business management platform. It gives a small business one friendly landing zone for links, relationships, follow-ups, documents, costs, integrations, and notifications.

The application is a Go API with a responsive React frontend, packaged as one stateless Cloud Run service that scales to zero. It includes Google login, roles, accounts and relationship management, account event timelines, Markdown knowledge, PDF signing, reminders, Gmail actions, shared Google Voice contact synchronization, Google Voice handoffs, Tiller imports, costs and receipts, notifications, audit history, exports, and configurable landing-zone shortcuts. See [plan.md](plan.md) for the product contract.

## Local development

```sh
make dev
```

The API runs on `http://localhost:8080`. The frontend development server runs on `http://localhost:5173`.

Google login requires `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, and a random `KOSMOS_SESSION_SECRET` of at least 32 bytes. `KOSMOS_INTEGRATION_SECRET` separately encrypts provider tokens and authenticates attachment and document signing links; local development falls back to the session secret when it is unset. The same `/auth/callback` handles login and optional incremental Google Workspace authorization. Production also requires `KOSMOS_ALLOWED_GOOGLE_DOMAINS`, a comma-separated list of verified email domains allowed to receive or retain a session. Configure `KOSMOS_GCP_PROJECT` and `KOSMOS_ATTACHMENTS_BUCKET` together for Firestore and private files, plus `KOSMOS_ORGANIZATION_ID` for the shared organization scope. Both web and jobs processes reject a configured GCP project without an attachment bucket. Without a GCP project, modules use in-memory adapters for local development. `KOSMOS_WEB_ROOT` can point the backend at a local frontend production build.

The production login policy currently permits verified Google accounts at `nerdswhofish.com`, `theoutdoorprogrammer.com`, and `apollorion.com`. This check happens at the callback and on every authenticated request, so removing a domain also invalidates its existing sessions.

## Deployment

`infra/modules/environment` is published to the Spacelift private registry as `kosmos-environment`. Environment roots consume `spacelift.stout.zone/theoutdoorprogrammer/kosmos-environment/google`, while keeping only project bootstrap, operator access, deployment inputs, and secret payloads outside the module.

The private jobs service receives the same `KOSMOS_ATTACHMENTS_BUCKET` as the web service. Its dedicated identity has only `storage.objects.delete`, restricted to the organization's `signing/` object prefix; it cannot read PDFs, list objects, or change retention. Cloud Run deployment waits for that bucket grant. Signing cleanup keeps its durable retry record when the bucket's 30-day retention prevents deletion.

Uploaded, prepared, and signed PDFs are encrypted at rest with Google-managed AES-256 keys and transferred to Cloud Storage over HTTPS. The bucket uses [standard Cloud Storage encryption](https://docs.cloud.google.com/storage/docs/encryption/default-keys). Active documents remain stored indefinitely until explicitly deleted. The 30-day retention setting provides minimum deletion protection. Automatic lifecycle cleanup removes only noncurrent versions after replacement or deletion; it never expires active documents.

Authenticated browser mutations require the `X-Kosmos-CSRF: 1` header in addition to the signed, HTTP-only session cookie. Public signing completion uses this header alongside its dedicated signing token. The versioned API contract lives at [api/openapi.yaml](api/openapi.yaml).

Production web processes also require a dedicated `KOSMOS_INTAKE_SECRET` of at least 32 bytes. The ingress proxy signs its verified client address for public contact intake; direct-origin intake fails closed. The environment module accepts this key through the ephemeral `intake_secret_value` input and provisions shared Firestore quotas with TTL cleanup. Local development without this key uses the socket peer address.

Owners and administrators can create named API credentials in Settings. The plaintext token is shown once and is sent as `Authorization: Bearer <token>`. Read-only credentials can inspect ordinary workspace APIs. Read-and-write credentials can mutate them, but credentials cannot manage members, other credentials, email, or provider integrations. Bearer-authenticated mutations do not use the browser CSRF header.

Repository workflows can converge a published document and its complete file set with `PUT /api/v1/managed-documents/{sourceKey}`. Send a multipart `document` JSON part and repeated `files` parts. Relative standard Markdown paths such as `assets/kosmos/logo.svg` resolve to same-document attachments by basename in Kosmos, while remaining valid in the source repository. The complete request shape is in the OpenAPI contract.

Provider setup, the public contact-form contract, Tiller headers, backup behavior, retention, and recovery are covered in [docs/operations.md](docs/operations.md).

## PDF signing

Open Documents, choose Signing, upload a PDF, and add up to ten signers with their names and emails. Place signature, date, name, or text fields on its pages and assign each field to a signer. Every signer needs a required signature field. Choose a link expiry from 1 to 90 days, then create and copy each person's link. Everyone can sign immediately. Creating links locks the recipients and fields; Kosmos does not email the links automatically.

Drag a field to move it and use its corner handles to resize it, with a mouse or touch. Fields stay within the page. Keyboard arrows adjust the focused field or resize handle; exact percentage controls are available under Precise position.

Each recipient opens their link without a Kosmos account, reviews the PDF, fills their assigned fields, and explicitly agrees to electronic signing. Signature fields accept typed text. Date fields use that person's signing date in UTC and name fields use their entered full name. Until everyone signs, downloads are marked partially signed. The final PDF includes everyone's signatures and signing records. Rotating the integration key invalidates all signing links, including completed downloads. Workspace users retain authenticated access after link expiry or key rotation.

Each original link remains available for 15 minutes after its recipient signs, independently of the other recipients. They can download the latest partial or final copy during that window. The same deadline applies to previously completed single-signer documents. Workspace users can create a fresh download-only link after everyone signs, choosing an expiry of up to seven days. Creating another download link replaces the previous generated link and preserves the signed PDF and its evidence.

Draft, signed, and revoked documents can be deleted after confirmation. Revoke a pending request before deleting it. Deletion immediately removes the document from the workspace and disables every link, then queues its private files for cleanup. Production bucket versioning retains deleted copies as noncurrent versions for its existing recovery period; failed storage operations remain queued for retry. See [ADR 0020](adr/0020-separate-signing-and-download-access-and-persist-document-cl.md).

Before consent, the signing page explains that signing records the person's IP address, browser description, and available approximate city, region, and country. These details are saved once per person and included in the PDF shared with the sender and other signers. Location comes from Cloudflare's IP-based estimate; the browser description is self-reported. Neither verifies identity. Kosmos does not collect GPS coordinates, fingerprints, cookies, session tokens, or document-view history for this audit. See [ADR 0021](adr/0021-collect-parallel-signatures-with-recipient-scoped-capabiliti.md) for parallel signing and evidence handling.

Uploads support unencrypted PDFs up to 10 MiB and 50 pages, with up to 100 fields. Already compatible PDFs remain unchanged. Kosmos automatically prepares other supported PDFs as static page images, preserving their visible form and annotation state while removing active content. Converted document bodies lose selectable text and accessibility structure, so review the prepared preview before creating a link. Encrypted PDFs, XFA, unsupported form states, and documents exceeding conversion limits are rejected. Signing text supports Windows-1252 Western European characters and must fit its field at a readable size.

A signing link proves possession, can be forwarded, and does not verify the recipient's identity or email. The signing record contains consent, the supplied name and email, a UTC timestamp, and the prepared document hash. For converted uploads, Kosmos retains the uploaded bytes for authenticated download and records their hash separately in the evidence. Metadata also records the completed PDF hash. This feature provides electronic signatures without a PKI digital certificate or a claim of DocuSign compliance equivalence. See [ADR 0017](adr/0017-embed-single-recipient-electronic-signing-in-kosmos.md), [ADR 0018](adr/0018-prepare-unsupported-signing-pdfs-as-static-page-images.md), and [operations](docs/operations.md#pdf-signing) for tradeoffs and recovery.

## Observability

The backend accepts standard `OTEL_EXPORTER_OTLP_*` environment variables and exports correlated logs and traces over OTLP HTTP when an endpoint is configured. Without an endpoint, telemetry remains local. Browser RUM and tracing use the public runtime configuration returned by `/api/v1/config` when `KOSMOS_FARO_URL` is present.

W3C trace context and baggage propagate across HTTP and queued jobs. Releases identify their version in telemetry, health, and browser configuration. SIGTERM drains active requests for up to seven seconds, then flushes telemetry with a separate two-second timeout. Browser telemetry retains route templates, timings, status codes, and anonymous context while removing customer queries, record identifiers, and free-text payloads before transport.

## License

MIT. See [LICENSE](LICENSE). Embedded PDF preparation components retain their own licenses in [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt), included in release archives and container images.
