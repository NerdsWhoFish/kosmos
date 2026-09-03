# Kosmos agent instructions

Kosmos is being built from the decisions and delivery slices in [plan.md](plan.md). Read that file before changing product scope, architecture, API contracts, infrastructure, or module boundaries.

## Working rules

- Keep the product approachable for nontechnical small-business users.
- Preserve the Go backend and React frontend split.
- Extend through module contracts, registries, typed events, provider ports, and versioned APIs.
- Do not add Kosmos-managed passwords. Identity is Google OAuth/OIDC first, with organization roles enforced server-side.
- Keep the default deployment stateless and near-free on GCP: Cloud Run scales to zero, durable data lives behind explicit managed services, and asynchronous work uses queues.
- Treat `infra/tofu/modules/kosmos-cloud-run` as a reusable module. The `configurations` repository is responsible for composing environments and calling it through Spacelift.
- Keep Dracula Classic tokens as the default UI theme, with semantic CSS variables so organizations can supply another theme without rewriting components.
- Instrument backend requests and future jobs with OpenTelemetry. Keep the Faro boundary in the frontend. Never send secrets, email bodies, receipts, or raw customer PII to telemetry.
- Update `plan.md` when a new capability or architectural decision is introduced.

## Verification

Run `GOWORK=off go test ./...`, `npm --prefix frontend test`, and `npm --prefix frontend run build` for code changes. Releases must use the Quill workflow in `.github/workflows/release.yml`. For infrastructure changes, run `tofu fmt -check` and `tofu validate` from the module directory when the provider is available.
