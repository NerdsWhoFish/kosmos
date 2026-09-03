# Kosmos

Kosmos is an MIT-licensed, extensible small business management platform. It gives a small business one friendly landing zone for links, relationships, follow-ups, documents, costs, integrations, and notifications.

The first implementation is a Go API with a React frontend, packaged as a stateless Cloud Run service that can scale to zero. See [plan.md](plan.md) for the product and architecture roadmap.

## Local development

```sh
make dev
```

The API runs on `http://localhost:8080`. The frontend development server runs on `http://localhost:5173`.

## Observability

The backend accepts standard `OTEL_EXPORTER_OTLP_*` environment variables and exports traces over OTLP HTTP when an endpoint is configured. Without an endpoint, tracing remains a no-op locally. Browser RUM is initialized through the frontend telemetry boundary when `VITE_FARO_URL` is present.

## License

MIT. See [LICENSE](LICENSE).
