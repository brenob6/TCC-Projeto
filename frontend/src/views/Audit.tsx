import { useEffect, useState } from 'react'
import type { ViewProps } from '../App.tsx'
import { Badge, Card, Field, type Tone } from '../components/ui.tsx'
import type { AccessLogAsset, AssetRef, Operation, PatientRecord } from '../types.ts'
import { OPERATIONS, errorMessage, formatDate, names, short } from '../util.ts'

const TONE: Record<Operation, Tone> = {
  CRIACAO: 'blue',
  LEITURA: 'green',
  LEITURA_EMERGENCIAL: 'amber',
  SOLICITACAO_EMERGENCIAL: 'amber',
  CONCESSAO: 'green',
  REVOGACAO: 'neutral',
  INVALIDACAO: 'red',
  ACESSO_NEGADO: 'red',
}

interface Filter {
  record: string
  from: string
  to: string
}

const emptyFilter: Filter = { record: '', from: '', to: '' }

// Trilha de auditoria (RF-03, RF-08): quem acessou, de qual instituição, qual
// operação e quando. Inclui leituras negadas e emergenciais.
export default function Audit({ api, org, institution, directory }: ViewProps) {
  const patients = directory?.patients || []
  const [patientKey, setPatientKey] = useState('')
  const [records, setRecords] = useState<PatientRecord[]>([])
  const [filter, setFilter] = useState<Filter>(emptyFilter)
  const [history, setHistory] = useState<AccessLogAsset[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const nameOf = names(directory)

  useEffect(() => { if (!patientKey && patients[0]) setPatientKey(patients[0].key) }, [directory]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    setFilter(emptyFilter)
    if (patientKey && institution) api.allRecords(patientKey).then(setRecords).catch(() => setRecords([]))
  }, [api, patientKey, institution])

  async function load() {
    const q: Record<string, string> = {}
    if (filter.record) q.record = filter.record
    if (filter.from) q.from = new Date(filter.from).toISOString()
    if (filter.to) q.to = new Date(filter.to).toISOString()
    try {
      setHistory(await api.history(patientKey, q))
      setError(null)
    } catch (err) {
      setError(errorMessage(err))
      setHistory(null)
    }
  }

  useEffect(() => { if (patientKey && institution) load() }, [patientKey, filter.record, institution]) // eslint-disable-line react-hooks/exhaustive-deps

  if (!institution) return <Card title="Auditoria"><p className="muted">Cadastre a instituição da {org} na aba Instituição.</p></Card>

  const titleOf = (ref: AssetRef) => records.find((r) => r.key === ref['@key'])?.title

  return (
    <Card title="Histórico de acessos" subtitle="getAccessHistory: registros somente de inserção, consultados por paciente ou por prontuário, com filtro de período.">
      <div className="grid">
        <Field label="Paciente">
          <select value={patientKey} onChange={(e) => setPatientKey(e.target.value)}>
            {patients.map((p) => <option key={p.key} value={p.key}>{p.name} — {short(p.id, 8)}</option>)}
          </select>
        </Field>
        <Field label="Prontuário">
          <select value={filter.record} onChange={(e) => setFilter({ ...filter, record: e.target.value })}>
            <option value="">Todos</option>
            {records.map((r) => <option key={r.cid} value={r.cid}>{r.title}</option>)}
          </select>
        </Field>
        <Field label="De"><input type="datetime-local" value={filter.from} onChange={(e) => setFilter({ ...filter, from: e.target.value })} /></Field>
        <Field label="Até"><input type="datetime-local" value={filter.to} onChange={(e) => setFilter({ ...filter, to: e.target.value })} /></Field>
        <div className="form-actions"><button onClick={load}>Consultar</button></div>
      </div>
      {error && <p className="error">{error}</p>}
      {history && (history.length === 0 ? <p className="muted">Nenhum registro.</p> : (
        <table>
          <thead><tr><th>Momento</th><th>Operação</th><th>Responsável</th><th>Instituição</th><th>MSP</th><th>Prontuário</th><th>Detalhes</th><th>Transação</th></tr></thead>
          <tbody>
            {history.map((h) => (
              <tr key={h['@key']}>
                <td>{formatDate(h.occurredAt)}</td>
                <td><Badge tone={TONE[h.operation]}>{OPERATIONS[h.operation] || h.operation}</Badge></td>
                <td>{h.actor ? nameOf(h.actor) : <span className="muted">o próprio paciente</span>}</td>
                <td>{nameOf(h.institution)}</td>
                <td className="mono small">{h.callerMsp}</td>
                <td>{h.record ? (titleOf(h.record) || <span className="mono small">{short(h.record['@key'], 18)}</span>) : '—'}</td>
                <td>{h.details || '—'}</td>
                <td className="mono small" title={h.id}>{short(h.id, 10)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ))}
    </Card>
  )
}
