import {
  getWebInstrumentations,
  initializeFaro,
  TransportItemType,
  type BrowserConfig,
  type EventEvent,
  type ExceptionEvent,
  type LogEvent,
  type MeasurementEvent,
  type TraceEvent,
  type TransportItem,
} from '@grafana/faro-web-sdk'
import { TracingInstrumentation } from '@grafana/faro-web-tracing'

const resources = new Set([
  'accounts', 'contacts', 'contact-sources', 'opportunities', 'documents', 'costs',
  'activities', 'reminders', 'attachments', 'notifications', 'members', 'credentials', 'signing', 'signing-requests',
])
const pages = new Set(['search', 'activity', 'communications', 'operations', 'settings', 'intake', 'sign'])
const endpoints = new Set(['search', 'summary', 'landing', 'config', 'modules', 'session', 'me'])
const actions = new Set(['edit', 'delete', 'revisions', 'download', 'photo', 'sync', 'read', 'pdf', 'link', 'revoke', 'complete'])
const eventNames = new Set([
  'click', 'navigation', 'view_changed', 'session_start', 'session_resume', 'session_extend',
  'route_change', 'faro.navigation', 'faro.performance.navigation', 'faro.performance.resource',
  'faro.tracing.fetch', 'faro.tracing.xml-http-request', 'faro.user.action', 'securitypolicyviolation',
])
const methods = new Set(['GET', 'HEAD', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS'])
const urlAttributes = new Set(['url', 'http.url', 'url.full', 'http.target', 'fromUrl', 'toUrl', 'name', 'documentURL', 'blockedURL'])
const numericAttributes = new Set([
  'http.status_code', 'http.response.status_code', 'http.request_content_length_uncompressed',
  'http.response_content_length', 'http.response_content_length_uncompressed', 'duration_ns',
  'duration', 'responseStatus', 'tcpHandshakeTime', 'dnsLookupTime', 'tlsNegotiationTime',
  'redirectTime', 'requestTime', 'responseTime', 'fetchTime', 'serviceWorkerTime', 'decodedBodySize',
  'encodedBodySize', 'ttfb', 'transferSize', 'pageLoadTime', 'documentParsingTime', 'domProcessingTime',
  'domContentLoadHandlerTime', 'onLoadTime',
])
const metricNames = new Set([
  'cls', 'fcp', 'inp', 'lcp', 'ttfb', 'fid', 'largest_shift_value', 'largest_shift_time',
  'first_byte_to_fcp', 'time_to_first_byte', 'interaction_time', 'presentation_delay', 'input_delay',
  'processing_duration', 'next_paint_time', 'total_script_duration', 'total_style_and_layout_duration',
  'total_paint_duration', 'total_unattributed_duration', 'longest_script_intersecting_duration',
  'element_render_delay', 'resource_load_delay', 'resource_load_duration', 'dns_duration',
  'connection_duration', 'request_duration', 'waiting_duration', 'cache_duration',
])
const errorTypes = new Set(['Error', 'TypeError', 'RangeError', 'ReferenceError', 'SyntaxError', 'URIError', 'EvalError', 'UnhandledRejection'])

export function telemetryURL(value: string): string {
  try {
    const url = new URL(value, window.location.origin)
    if (url.origin !== window.location.origin) return 'https://external.invalid/:path'
    const api = url.pathname.startsWith('/api/v1/')
    const parts = url.pathname.slice(api ? '/api/v1/'.length : 1).split('/').filter(Boolean)
    let path = '/:path'
    if (!parts.length) path = '/'
    else if (resources.has(parts[0])) {
      path = `/${parts[0]}`
      if (parts.length > 1) path += !api && parts[1] === 'new' ? '/new' : '/:id'
      if (parts.length > 2) path += actions.has(parts[2]) ? `/${parts[2]}` : '/:path'
      if (parts.length > 3) path += '/:path'
    } else if ((api ? endpoints : pages).has(parts[0])) {
      path = `/${parts[0]}`
      if (!api && parts[0] === 'activity' && parts[1] === 'future' && parts.length === 2) path += '/future'
      else if (parts.length > 1) path += '/:path'
    } else if (parts[0] === 'assets') path = '/assets/:asset'
    else if (parts[0] === 'auth' && ['login', 'logout', 'callback'].includes(parts[1])) path = `/auth/${parts[1]}`
    return `${url.origin}${api ? '/api/v1' : ''}${path}`
  } catch {
    return `${window.location.origin}/:path`
  }
}

function safeAttribute(key: string, value: unknown): string | number | undefined {
  if (typeof value === 'string' && urlAttributes.has(key)) return telemetryURL(value)
  if (['http.method', 'http.request.method'].includes(key) && typeof value === 'string' && methods.has(value)) return value
  if (numericAttributes.has(key) && (typeof value === 'number' || typeof value === 'string') && /^\d+(\.\d+)?$/.test(String(value)) && Number.isFinite(Number(value))) return value
  return undefined
}

function safeAttributes(attributes: Record<string, string> = {}): Record<string, string> {
  return Object.fromEntries(Object.entries(attributes).flatMap(([key, value]) => {
    const safe = safeAttribute(key, value)
    return safe === undefined ? [] : [[key, String(safe)]]
  }))
}

function safeTraces(payload: TraceEvent, app: TransportItem['meta']['app']): TraceEvent {
  return {
    resourceSpans: payload.resourceSpans?.map((resource) => ({
      resource: {
        attributes: [
          { key: 'service.name', value: { stringValue: app?.name ?? 'kosmos' } },
          { key: 'service.version', value: { stringValue: app?.version ?? 'development' } },
          { key: 'deployment.environment.name', value: { stringValue: app?.environment ?? 'unknown' } },
        ],
        droppedAttributesCount: 0,
      },
      scopeSpans: resource.scopeSpans.map((scope) => ({
        scope: { name: 'kosmos.browser' },
        spans: scope.spans?.map((span) => ({
          traceId: span.traceId,
          spanId: span.spanId,
          parentSpanId: span.parentSpanId,
          name: span.kind === 3 ? 'HTTP request' : 'browser operation',
          kind: span.kind,
          startTimeUnixNano: span.startTimeUnixNano,
          endTimeUnixNano: span.endTimeUnixNano,
          attributes: span.attributes.flatMap(({ key, value }) => {
            const safe = safeAttribute(key, value.stringValue ?? value.intValue ?? value.doubleValue)
            return safe === undefined ? [] : [{ key, value: typeof safe === 'number' ? { doubleValue: safe } : { stringValue: safe } }]
          }),
          droppedAttributesCount: span.droppedAttributesCount,
          events: span.events.map((event) => ({
            name: event.name === 'exception' ? 'exception' : 'browser event',
            timeUnixNano: event.timeUnixNano,
            attributes: [],
            droppedAttributesCount: event.droppedAttributesCount,
          })),
          droppedEventsCount: span.droppedEventsCount,
          links: [],
          droppedLinksCount: span.droppedLinksCount,
          status: { code: span.status.code },
        })),
      })),
    })),
  }
}

// Unknown SDK fields must stay private, including events derived from spans.
export function sanitizeTelemetry(item: TransportItem): TransportItem | null {
  const meta = {
    app: item.meta.app ? { name: item.meta.app.name, version: item.meta.app.version, environment: item.meta.app.environment } : undefined,
    sdk: item.meta.sdk,
    session: item.meta.session ? {
      id: item.meta.session.id,
      // Faro's later sampling hook needs this flag and removes it before export.
      attributes: item.meta.session.attributes?.isSampled === 'true' ? { isSampled: 'true' } : undefined,
    } : undefined,
    page: { url: telemetryURL(item.meta.page?.url ?? window.location.href) },
    browser: item.meta.browser ? {
      name: item.meta.browser.name,
      version: item.meta.browser.version,
      mobile: item.meta.browser.mobile,
      viewportWidth: item.meta.browser.viewportWidth,
      viewportHeight: item.meta.browser.viewportHeight,
    } : undefined,
  }
  switch (item.type) {
    case TransportItemType.TRACE:
      return { type: item.type, meta, payload: safeTraces(item.payload as TraceEvent, meta.app) }
    case TransportItemType.EVENT: {
      const payload = item.payload as EventEvent
      return { type: item.type, meta, payload: {
        name: eventNames.has(payload.name) ? payload.name : 'browser event',
        timestamp: payload.timestamp,
        trace: payload.trace,
        attributes: safeAttributes(payload.attributes),
      } }
    }
    case TransportItemType.EXCEPTION: {
      const payload = item.payload as ExceptionEvent
      return { type: item.type, meta, payload: {
        type: errorTypes.has(payload.type) ? payload.type : 'Error',
        value: 'Browser error',
        timestamp: payload.timestamp,
        trace: payload.trace,
        fatal: payload.fatal,
        stacktrace: payload.stacktrace ? { frames: payload.stacktrace.frames.map((frame) => ({
          filename: telemetryURL(frame.filename),
          function: '(anonymous)',
          lineno: frame.lineno,
          colno: frame.colno,
        })) } : undefined,
      } }
    }
    case TransportItemType.LOG: {
      const payload = item.payload as LogEvent
      return { type: item.type, meta, payload: {
        level: payload.level,
        message: 'Browser console message',
        timestamp: payload.timestamp,
        trace: payload.trace,
        context: undefined,
      } }
    }
    case TransportItemType.MEASUREMENT: {
      const payload = item.payload as MeasurementEvent
      const values = Object.fromEntries(Object.entries(payload.values).filter(([key, value]) => metricNames.has(key) && Number.isFinite(value)))
      if (!Object.keys(values).length) return null
      return { type: item.type, meta, payload: { type: 'web-vitals', values, timestamp: payload.timestamp, trace: payload.trace } }
    }
    default:
      return null
  }
}

export function telemetryConfig(url: string, appName = 'kosmos', version = 'development'): BrowserConfig {
  return {
    url,
    app: { name: appName, version, environment: import.meta.env.MODE },
    beforeSend: sanitizeTelemetry,
    instrumentations: [...getWebInstrumentations({ captureConsole: true }), new TracingInstrumentation()],
  }
}

export function initializeTelemetry(url: string, appName = 'kosmos', version = 'development') {
  if (!url) return
  return initializeFaro(telemetryConfig(url, appName, version))
}
