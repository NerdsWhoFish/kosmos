import { describe, expect, it } from 'vitest'
import { BaseTransport, initializeFaro, type TransportItem, TransportItemType } from '@grafana/faro-web-sdk'
import { FaroTraceExporter, TracingInstrumentation } from '@grafana/faro-web-tracing'
import { WebTracerProvider, SimpleSpanProcessor } from '@opentelemetry/sdk-trace-web'
import { SpanKind, SpanStatusCode } from '@opentelemetry/api'
import { initializeTelemetry, telemetryConfig, telemetryURL } from './telemetry'

class CaptureTransport extends BaseTransport {
  name = 'test-capture'
  version = '1'
  items: TransportItem[] = []
  initialize() {}
  send(items: TransportItem | TransportItem[]) {
    this.items.push(...JSON.parse(JSON.stringify(Array.isArray(items) ? items : [items])))
  }
}

describe('browser telemetry', () => {
  it('stays disabled without a collector', () => {
    expect(initializeTelemetry('')).toBeUndefined()
  })

  it.each([
    ['/search?q=customer%40example.com#secret', '/search'],
    ['/api/v1/search?q=customer%40example.com', '/api/v1/search'],
    ['/documents/customer%40example.com/edit?token=secret', '/documents/:id/edit'],
    ['/api/v1/attachments/private-id/download?token=secret', '/api/v1/attachments/:id/download'],
    ['/unknown/customer%40example.com', '/:path'],
    ['/contacts/new', '/contacts/new'],
    ['/api/v1/contacts/new', '/api/v1/contacts/:id'],
    ['/sign#private-request.secret-signing-token', '/sign'],
    ['/api/v1/signing/private-request/complete', '/api/v1/signing/:id/complete'],
    ['/api/v1/signing-requests/private-request/pdf?completed=true', '/api/v1/signing-requests/:id/pdf'],
  ])('normalizes %s into a bounded route', (input, expected) => {
    expect(telemetryURL(input)).toBe(`${location.origin}${expected}`)
  })

  it('sanitizes real Faro transport payloads and the real OTel exporter', async () => {
    const secret = 'synthetic-customer@example.com'
    const secretID = 'private-record-id'
    const url = `${location.origin}/api/v1/contacts/${secretID}?q=${encodeURIComponent(secret)}#secret-fragment`
    window.history.replaceState({}, '', `/search?q=${encodeURIComponent(secret)}`)
    const transport = new CaptureTransport()
    const config = telemetryConfig('https://collector.example/collect', 'kosmos', 'abc123')
    expect(config.instrumentations?.some((item) => item instanceof TracingInstrumentation)).toBe(true)
    const faro = initializeFaro({
      ...config,
      url: undefined,
      transports: [transport],
      batching: { enabled: false },
      isolate: true,
      preventGlobalExposure: true,
      sessionTracking: { persistent: false },
      instrumentations: config.instrumentations?.filter((item) => !(item instanceof TracingInstrumentation)),
    })
    const provider = new WebTracerProvider({ spanProcessors: [new SimpleSpanProcessor(new FaroTraceExporter({ api: faro.api }))] })
    try {
      faro.api.setUser({ email: secret, id: secretID, fullName: secret, attributes: { note: secret } })
      faro.api.setView({ name: secret })
      faro.metas.add({ page: { url: window.location.href, attributes: { title: secret } } })
      faro.metas.add({ session: {
        ...faro.metas.value.session,
        attributes: { ...faro.metas.value.session?.attributes, note: secret },
      } })
      faro.api.pushEvent('faro.performance.resource', { name: url, httpHost: secret, duration: '32', responseStatus: '200' })
      faro.api.pushEvent('faro.navigation', { fromUrl: url, toUrl: window.location.href, duration: '18', text: secret })
      faro.api.pushEvent(secret, { 'faro.action.user.name': secret, name: secret, form: secret })
      faro.api.pushEvent('click', {}, undefined, {
        customPayloadTransformer: (payload) => ({ ...payload, action: { name: secret, parentId: secretID } }),
      })
      faro.api.pushLog([secret], { context: { contact: secret } })
      faro.api.pushError(new TypeError(secret), { context: { contact: secret } })
      faro.api.pushMeasurement({ type: 'web-vitals', values: { lcp: 42, [secret]: 99 } }, { context: { element: secret } })

      const span = provider.getTracer('@opentelemetry/instrumentation-fetch').startSpan(`GET ${url}`, {
        kind: SpanKind.CLIENT,
        attributes: { 'http.url': url, 'http.method': 'GET', 'http.status_code': 503, 'user.email': secret, 'http.request.header.authorization': secret },
      })
      span.setStatus({ code: SpanStatusCode.ERROR, message: secret })
      span.recordException(new Error(secret))
      span.addEvent(secret, { note: secret })
      span.end()
      await provider.forceFlush()

      const exported = JSON.stringify(transport.items)
      expect(exported).not.toContain(secret)
      expect(exported).not.toContain(encodeURIComponent(secret))
      expect(exported).not.toContain(secretID)
      expect(exported).not.toContain('secret-fragment')
      expect(exported).not.toContain('authorization')
      expect(exported).toContain('/api/v1/contacts/:id')
      expect(exported).toContain('faro.tracing.fetch')
      expect(exported).toContain('HTTP request')
      expect(exported).toContain('503')
      expect(exported).toContain('abc123')
      expect(transport.items.some((item) => item.type === TransportItemType.EXCEPTION)).toBe(true)
      expect(transport.items.some((item) => item.type === TransportItemType.TRACE)).toBe(true)
      expect(transport.items.some((item) => item.type === TransportItemType.MEASUREMENT)).toBe(true)
      for (const item of transport.items) {
        expect(item.meta.page?.url).toBe(`${location.origin}/search`)
        expect(item.meta.user).toBeUndefined()
        expect(item.meta.view).toBeUndefined()
      }
    } finally {
      await provider.shutdown()
      faro.instrumentations.remove(...faro.instrumentations.instrumentations)
      faro.pause()
      window.history.replaceState({}, '', '/')
    }
  })
})
