import { getWebInstrumentations, initializeFaro } from '@grafana/faro-web-sdk'
import { TracingInstrumentation } from '@grafana/faro-web-tracing'

export function initializeTelemetry(url: string, appName = 'kosmos') {
  if (!url) return

  initializeFaro({
    url,
    app: { name: appName, version: '0.1.0', environment: import.meta.env.MODE },
    instrumentations: [...getWebInstrumentations({ captureConsole: true }), new TracingInstrumentation()],
  })
}
