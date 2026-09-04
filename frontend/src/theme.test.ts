import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const styles = readFileSync('src/styles.css', 'utf8')
const compactStyles = styles.replace(/\s+/g, ' ')
const indexHTML = readFileSync('index.html', 'utf8')

function cssColor(name: string) {
  const match = styles.match(new RegExp(`--${name}:\\s*(#[0-9A-Fa-f]{6})`))
  if (!match) throw new Error(`Missing CSS color token: ${name}`)
  return match[1]
}

function luminance(color: string) {
  const channels = color.match(/[0-9a-f]{2}/gi)
  if (!channels) throw new Error(`Invalid color: ${color}`)

  const [red, green, blue] = channels.map((channel) => {
    const value = Number.parseInt(channel, 16) / 255
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  })

  return 0.2126 * red + 0.7152 * green + 0.0722 * blue
}

function contrast(foreground: string, background: string) {
  const lighter = Math.max(luminance(foreground), luminance(background))
  const darker = Math.min(luminance(foreground), luminance(background))
  return (lighter + 0.05) / (darker + 0.05)
}

describe('Dracula theme accessibility', () => {
  it('keeps muted text readable across dashboard surfaces', () => {
    expect(styles).toContain('--theme-muted')
    const muted = cssColor('theme-muted')
    const surfaces = [
      cssColor('dracula-background'),
      cssColor('theme-surface'),
      cssColor('theme-surface-raised'),
      cssColor('theme-surface-dark'),
    ]

    for (const surface of surfaces) {
      expect(contrast(muted, surface)).toBeGreaterThanOrEqual(4.5)
    }
  })
})

describe('mobile form accessibility', () => {
  it('keeps browser zoom available while preventing iOS focus zoom', () => {
    expect(indexHTML).toContain('width=device-width, initial-scale=1, viewport-fit=cover')
    expect(indexHTML).not.toMatch(/maximum-scale|user-scalable/)
    expect(compactStyles).toContain(
      '.app-shell input:not([type="checkbox"]), .app-shell select, .app-shell textarea { font-size: 16px; }',
    )
  })

  it('stacks panel labels and actions instead of squeezing fields together', () => {
    expect(compactStyles).toContain('.panel form label { min-width: 0; display: grid;')
    expect(compactStyles).toContain('.panel .form-actions { grid-template-columns: 1fr; }')
  })
})
