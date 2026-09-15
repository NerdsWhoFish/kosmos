import { Component, type ReactNode } from 'react'
import { faro } from '@grafana/faro-web-sdk'
import { ErrorState } from './States'

export class PageErrorBoundary extends Component<{ children: ReactNode; resetKey: string }, { failed: boolean; resetKey: string }> {
  state = { failed: false, resetKey: this.props.resetKey }

  static getDerivedStateFromProps(props: { resetKey: string }, state: { resetKey: string }) {
    return props.resetKey === state.resetKey ? null : { failed: false, resetKey: props.resetKey }
  }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  componentDidCatch(error: Error) {
    faro?.api.pushError(error)
  }

  render() {
    if (this.state.failed) {
      return <ErrorState
        message="This page could not open. Reload the page to try again."
        retryLabel="Reload page"
        // Reload clears React and browser caches of failed imports.
        retry={() => window.location.reload()}
      />
    }
    return this.props.children
  }
}
