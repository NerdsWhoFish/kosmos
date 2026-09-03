import { beforeEach, describe, expect, it, vi } from 'vitest'
import { initializeFaro } from '@grafana/faro-web-sdk'
import { initializeTelemetry } from './telemetry'

vi.mock('@grafana/faro-web-sdk', () => ({
  getWebInstrumentations: vi.fn(() => ['web']),
  initializeFaro: vi.fn(),
}))

vi.mock('@grafana/faro-web-tracing', () => ({
  TracingInstrumentation: class TracingInstrumentation {},
}))

describe('browser telemetry', () => {
  beforeEach(() => vi.mocked(initializeFaro).mockClear())

  it('stays disabled without a collector', () => {
    initializeTelemetry('')
    expect(initializeFaro).not.toHaveBeenCalled()
  })

  it('uses runtime configuration and tracing', () => {
    initializeTelemetry('https://faro.example.com/collect/test', 'kosmos')
    expect(initializeFaro).toHaveBeenCalledWith(expect.objectContaining({
      url: 'https://faro.example.com/collect/test',
      app: expect.objectContaining({ name: 'kosmos' }),
      instrumentations: expect.arrayContaining(['web', expect.anything()]),
    }))
  })
})
