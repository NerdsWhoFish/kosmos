import { getWebInstrumentations, initializeFaro } from '@grafana/faro-web-sdk'

export function initializeTelemetry() {
  const url = import.meta.env.VITE_FARO_URL
  if (!url) return

  initializeFaro({
    url,
    app: { name: 'kosmos', version: '0.1.0', environment: import.meta.env.MODE },
    instrumentations: [...getWebInstrumentations()],
  })
}
