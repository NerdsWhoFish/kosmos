import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './app/App'
import { initializeTelemetry } from './telemetry'
import './styles.css'

async function bootstrap() {
  try {
    const response = await fetch('/api/v1/config')
    if (response.ok) {
      const config = await response.json()
      initializeTelemetry(config.faroURL, config.faroAppName, config.faroVersion)
    }
  } catch {
    // Telemetry must never prevent the workspace from loading.
  }

  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
}

void bootstrap()
