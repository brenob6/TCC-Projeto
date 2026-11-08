import { useState, type ReactNode } from 'react'
import { ApiError } from '../api.ts'
import type { TraceStep, TxResponse } from '../types.ts'
import { errorMessage } from '../util.ts'

export function Card({ title, subtitle, actions, children }: {
  title?: ReactNode
  subtitle?: ReactNode
  actions?: ReactNode
  children?: ReactNode
}) {
  return (
    <section className="card">
      {(title || actions) && (
        <header className="card-header">
          <div>
            {title && <h2>{title}</h2>}
            {subtitle && <p className="muted">{subtitle}</p>}
          </div>
          {actions && <div className="row">{actions}</div>}
        </header>
      )}
      {children}
    </section>
  )
}

export function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small className="muted">{hint}</small>}
    </label>
  )
}

export type Tone = 'neutral' | 'blue' | 'green' | 'amber' | 'red'

export function Badge({ tone = 'neutral', children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`badge badge-${tone}`}>{children}</span>
}

const LAYER_TONE: Record<TraceStep['layer'], Tone> = { Blockchain: 'blue', IPFS: 'amber', KMS: 'green', API: 'neutral' }

// Trace exibe as interações da API com cada camada (Figuras 7 a 9).
export function Trace({ steps }: { steps?: TraceStep[] }) {
  if (!steps?.length) return null
  return (
    <ol className="trace">
      {steps.map((s, i) => (
        <li key={i}>
          <Badge tone={LAYER_TONE[s.layer]}>{s.layer}</Badge>
          <div>
            <strong>{s.action}</strong>
            {s.detail && <span className="muted"> — {s.detail}</span>}
            {s.txId && <div className="mono small">tx {s.txId}</div>}
          </div>
        </li>
      ))}
    </ol>
  )
}

export interface ActionState {
  message?: string
  error?: string
  txId?: string
  trace?: TraceStep[]
}

export function Outcome({ state }: { state: ActionState | null }) {
  if (!state) return null
  return (
    <div className={`outcome ${state.error ? 'outcome-error' : 'outcome-ok'}`}>
      <p>{state.error || state.message}</p>
      {state.txId && <p className="mono small">tx {state.txId}</p>}
      <Trace steps={state.trace} />
    </div>
  )
}

// useAction executa uma operação assíncrona e guarda o resultado para exibição.
export function useAction() {
  const [busy, setBusy] = useState(false)
  const [state, setState] = useState<ActionState | null>(null)
  async function run<T>(fn: () => Promise<T>, message?: string | ((res: T) => string)): Promise<T | undefined> {
    setBusy(true)
    setState(null)
    try {
      const res = await fn()
      const msg = typeof message === 'function' ? message(res) : message
      const tx = res as TxResponse
      setState({ message: msg || 'Operação concluída.', txId: tx.txId, trace: tx.trace })
      return res
    } catch (err) {
      setState({ error: errorMessage(err), trace: err instanceof ApiError ? err.trace : undefined })
      return undefined
    } finally {
      setBusy(false)
    }
  }
  return { busy, state, setState, run }
}

export function JsonView({ value }: { value: unknown }) {
  return <pre className="json">{JSON.stringify(value, null, 2)}</pre>
}
