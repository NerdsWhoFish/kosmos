# Kosmos

Kosmos is an MIT-licensed, extensible small business management platform. It gives a small business one friendly landing zone for links, relationships, follow-ups, documents, costs, integrations, and notifications.

The first implementation is a Go API with a React frontend, packaged as a stateless Cloud Run service that can scale to zero. See [plan.md](plan.md) for the product and architecture roadmap.

## Local development

```sh
make dev
```

The API runs on `http://localhost:8080`. The frontend development server runs on `http://localhost:5173`.

Google login requires `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, and a random `KOSMOS_SESSION_SECRET` of at least 32 bytes. Production also requires `KOSMOS_ALLOWED_GOOGLE_DOMAINS`, a comma-separated list of verified email domains allowed to receive a session. Set `KOSMOS_GCP_PROJECT` to use Firestore. Without it, the same landing-zone API uses an in-memory store for local development.

## Deployment

`infra/modules/environment` is published to the Spacelift private registry as `kosmos-environment`. Environment roots consume `spacelift.stout.zone/theoutdoorprogrammer/kosmos-environment/google`, while keeping only project bootstrap, operator access, deployment inputs, and secret payloads outside the module.

## Observability

The backend accepts standard `OTEL_EXPORTER_OTLP_*` environment variables and exports correlated logs and traces over OTLP HTTP when an endpoint is configured. Without an endpoint, telemetry remains local. Browser RUM and tracing use the public runtime configuration returned by `/api/v1/config` when `KOSMOS_FARO_URL` is present.

## License

MIT. See [LICENSE](LICENSE).
