import { useEffect, useState, type FormEvent } from 'react'
import type { ViewProps } from '../App.tsx'
import type { Api } from '../api.ts'
import { Badge, Card, Field, Outcome, useAction, type Tone } from '../components/ui.tsx'
import type {
  AccessLevel, Directory, EmergencyAsset, EmergencyStatus, PatientAsset, PatientRecord, PermissionAsset,
  PermissionStatus, Signature, TxResponse,
} from '../types.ts'
import { ACCESS_LEVELS, RECORD_TYPES, entries, formatDate, names, short } from '../util.ts'
import { formatTime, loadWallet, sign, signedMessage, type Json } from '../wallet.ts'

type SignAndSend = <T extends TxResponse>(
  action: string,
  payload: Record<string, Json>,
  send: (sig: Signature) => Promise<T>,
) => Promise<T & { message: string; signature: string }>

/** Contexto compartilhado pelos painéis da carteira. */
interface Context {
  api: Api
  patientKey: string
  directory: Directory | null
  seed: string | null
  signAndSend: SignAndSend
  version: number
}

// Carteira do paciente: as operações assinadas (concessão, revogação e
// eliminação) são montadas e assinadas neste navegador e apenas encaminhadas
// pela API da instituição selecionada.
export default function Patient({ api, org, institution, directory }: ViewProps) {
  const patients = directory?.patients || []
  const [patientKey, setPatientKey] = useState('')
  const [patient, setPatient] = useState<PatientAsset | null>(null)
  const [version, setVersion] = useState(0)
  const bump = () => setVersion((v) => v + 1)

  useEffect(() => { if (!patientKey && patients[0]) setPatientKey(patients[0].key) }, [directory]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    setPatient(null)
    if (patientKey && institution) api.patient(patientKey).then(setPatient).catch(() => {})
  }, [api, patientKey, institution, version])

  if (!institution) return <Card title="Paciente"><p className="muted">Cadastre a instituição da {org} na aba Instituição.</p></Card>

  const entry = patients.find((p) => p.key === patientKey)
  const seed = entry ? loadWallet(entry.id) : null

  // signAndSend obtém o nonce atual do ledger, assina a mensagem canônica e
  // envia a operação.
  const signAndSend: SignAndSend = async (action, payload, send) => {
    if (!seed) throw new Error('chave de assinatura do paciente ausente neste navegador')
    const current = await api.patient(patientKey)
    const nonce = (current.signatureNonce || 0) + 1
    const message = signedMessage(action, current.id, nonce, payload)
    const signature = sign(seed, message)
    const res = await send({ nonce, signature })
    bump()
    return { ...res, message, signature }
  }

  const ctx: Context = { api, patientKey, directory, seed, signAndSend, version }
  return (
    <>
      <Card title="Carteira do paciente" subtitle="A chave privada de assinatura fica apenas neste navegador. A instituição em uso apenas encaminha as operações assinadas.">
        <div className="grid">
          <Field label="Paciente">
            <select value={patientKey} onChange={(e) => setPatientKey(e.target.value)}>
              {patients.length === 0 && <option value="">Nenhum paciente cadastrado</option>}
              {patients.map((p) => <option key={p.key} value={p.key}>{p.name} — {short(p.id, 8)}</option>)}
            </select>
          </Field>
          <Field label="Chave de assinatura">
            {seed ? <Badge tone="green">disponível neste navegador</Badge> : <Badge tone="red">ausente: operações assinadas indisponíveis</Badge>}
          </Field>
        </div>
        {patient && (
          <dl className="facts">
            <dt>Identificador pseudônimo</dt><dd className="mono">{patient.id}</dd>
            <dt>Hash do CPF (HMAC)</dt><dd className="mono small">{patient.cpfHash}</dd>
            <dt>Chave no KMS</dt><dd className="mono">{patient.kmsKeyId}</dd>
            <dt>Último nonce aceito</dt><dd>{patient.signatureNonce}</dd>
          </dl>
        )}
      </Card>
      {patient && (
        <>
          <Grant {...ctx} />
          <Permissions {...ctx} />
          <Records {...ctx} />
          <Emergencies {...ctx} />
        </>
      )}
    </>
  )
}

type BeneficiaryKind = 'professional' | 'institution'

interface GrantState {
  kind: BeneficiaryKind
  beneficiary: string
  accessLevel: AccessLevel
  purpose: string
  records: string[]
  expiresAt: string
}

const emptyGrant: GrantState = { kind: 'professional', beneficiary: '', accessLevel: 'leitura', purpose: '', records: [], expiresAt: '' }

function Grant({ api, patientKey, directory, seed, signAndSend, version }: Context) {
  const [form, setForm] = useState<GrantState>(emptyGrant)
  const [records, setRecords] = useState<PatientRecord[]>([])
  const [signedMsg, setSignedMsg] = useState<string | null>(null)
  const action = useAction()

  useEffect(() => { api.allRecords(patientKey).then(setRecords).catch(() => setRecords([])) }, [api, patientKey, version])

  const options = form.kind === 'professional'
    ? (directory?.professionals || []).map((p) => ({ key: p.key, id: p.id, label: `${p.name} (${p.org})` }))
    : (directory?.orgs || []).flatMap((o) => o.institution
      ? [{ key: o.institution.key, id: o.institution.id, label: `${o.institution.name} (${o.name})` }]
      : [])
  const selected = options.find((o) => o.key === form.beneficiary)
  const scopeOptions = records.filter((r) => !r.invalidated)

  function toggleRecord(cid: string) {
    const set = new Set(form.records)
    if (set.has(cid)) set.delete(cid)
    else set.add(cid)
    setForm({ ...form, records: [...set] })
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!selected) return
    setSignedMsg(null)
    const expiresAt = form.expiresAt ? formatTime(new Date(form.expiresAt)) : ''
    const scope = form.accessLevel === 'leitura' ? [...form.records].sort() : []
    // Campos da mensagem assinada; o chaincode reconstrói o mesmo objeto.
    const payload = {
      professional: form.kind === 'professional' ? selected.id : '',
      institution: form.kind === 'institution' ? selected.id : '',
      accessLevel: form.accessLevel,
      purpose: form.purpose,
      records: scope,
      expiresAt,
    }
    const res = await action.run(() => signAndSend('grantPermission', payload, (sig) => api.grant(patientKey, {
      ...sig,
      [form.kind]: form.beneficiary,
      accessLevel: form.accessLevel,
      purpose: form.purpose,
      records: scope,
      expiresAt,
    })), 'Permissão concedida e registrada com a assinatura do paciente (RF-04).')
    if (res) {
      setSignedMsg(res.message)
      setForm(emptyGrant)
    }
  }

  return (
    <Card title="Conceder permissão" subtitle="A mensagem assinada contém beneficiário, nível, finalidade, escopo e validade; a API não consegue alterar nenhum desses campos.">
      <form onSubmit={submit} className="grid">
        <Field label="Beneficiário">
          <select value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value as BeneficiaryKind, beneficiary: '' })}>
            <option value="professional">Profissional</option>
            <option value="institution">Instituição inteira</option>
          </select>
        </Field>
        <Field label={form.kind === 'professional' ? 'Profissional' : 'Instituição'}>
          <select value={form.beneficiary} onChange={(e) => setForm({ ...form, beneficiary: e.target.value })} required>
            <option value="">Selecione…</option>
            {options.map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
          </select>
        </Field>
        <Field label="Nível de acesso">
          <select value={form.accessLevel} onChange={(e) => setForm({ ...form, accessLevel: e.target.value as AccessLevel })}>
            {entries(ACCESS_LEVELS).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </select>
        </Field>
        <Field label="Finalidade" hint="Obrigatória (LGPD, finalidade determinada)">
          <input value={form.purpose} onChange={(e) => setForm({ ...form, purpose: e.target.value })} placeholder="acompanhamento clínico" required />
        </Field>
        <Field label="Válida até (opcional)"><input type="datetime-local" value={form.expiresAt} onChange={(e) => setForm({ ...form, expiresAt: e.target.value })} /></Field>
        {form.accessLevel === 'leitura' && scopeOptions.length > 0 && (
          <Field label="Escopo (opcional)" hint="Sem seleção, vale para todos os prontuários">
            <div className="checks">
              {scopeOptions.map((r) => (
                <label key={r.cid} className="check">
                  <input type="checkbox" checked={form.records.includes(r.cid)} onChange={() => toggleRecord(r.cid)} /> {r.title}
                </label>
              ))}
            </div>
          </Field>
        )}
        <div className="form-actions"><button disabled={action.busy || !seed || !selected}>Assinar e conceder</button></div>
      </form>
      <Outcome state={action.state} />
      {signedMsg && (<><p className="muted small">Mensagem assinada (Ed25519):</p><pre className="json">{signedMsg}</pre></>)}
    </Card>
  )
}

const PERMISSION_TONE: Record<PermissionStatus, Tone> = { ativa: 'green', revogada: 'red', expirada: 'neutral' }

function Permissions({ api, patientKey, directory, seed, signAndSend, version }: Context) {
  const [list, setList] = useState<PermissionAsset[]>([])
  const action = useAction()
  const nameOf = names(directory)

  useEffect(() => { api.permissions(patientKey).then(setList).catch(() => setList([])) }, [api, patientKey, version])

  async function revoke(p: PermissionAsset) {
    await action.run(() => signAndSend('revokePermission', { permission: p.id },
      (sig) => api.revoke(p['@key'], sig)), 'Permissão revogada; o efeito é imediato.')
  }

  return (
    <Card title="Permissões concedidas" subtitle="Situação efetiva calculada no ledger (permissões vencidas aparecem como expiradas).">
      {list.length === 0 ? <p className="muted">Nenhuma permissão concedida.</p> : (
        <table>
          <thead><tr><th>Beneficiário</th><th>Nível</th><th>Finalidade</th><th>Escopo</th><th>Validade</th><th>Situação</th><th></th></tr></thead>
          <tbody>
            {list.map((p) => (
              <tr key={p['@key']}>
                <td>{p.professional ? nameOf(p.professional) : <>{nameOf(p.institution)} <Badge>instituição</Badge></>}</td>
                <td>{ACCESS_LEVELS[p.accessLevel]}</td>
                <td>{p.purpose}</td>
                <td>{p.records?.length ? `${p.records.length} prontuário(s)` : 'todos'}</td>
                <td>{p.expiresAt ? formatDate(p.expiresAt) : 'até revogação'}</td>
                <td><Badge tone={PERMISSION_TONE[p.status]}>{p.status}</Badge></td>
                <td>{p.status === 'ativa' && <button className="danger" onClick={() => revoke(p)} disabled={action.busy || !seed}>Revogar</button>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <Outcome state={action.state} />
    </Card>
  )
}

function Records({ api, patientKey, directory, seed, signAndSend, version }: Context) {
  const [list, setList] = useState<PatientRecord[]>([])
  const [reasons, setReasons] = useState<Record<string, string>>({})
  const action = useAction()
  const nameOf = names(directory)

  useEffect(() => { api.allRecords(patientKey).then(setList).catch(() => setList([])) }, [api, patientKey, version])

  async function invalidate(r: PatientRecord) {
    const reason = reasons[r.cid] || ''
    await action.run(() => signAndSend('invalidateMedicalRecord', { record: r.cid, reason },
      (sig) => api.invalidate(r.cid, { ...sig, reason })), 'Prontuário eliminado logicamente; leituras posteriores serão negadas (RF-09).')
  }

  return (
    <Card title="Meus prontuários" subtitle="Eliminação lógica a pedido do titular (LGPD, art. 18): o ledger marca o prontuário como eliminado e a API remove o envelope dos nós IPFS. A trilha de auditoria permanece.">
      {list.length === 0 ? <p className="muted">Nenhum prontuário registrado.</p> : (
        <table>
          <thead><tr><th>Título</th><th>Tipo</th><th>Autor</th><th>Origem</th><th>Registrado em</th><th>Motivo</th><th></th></tr></thead>
          <tbody>
            {list.map((r) => (
              <tr key={r.cid}>
                <td>{r.title}<div className="mono small muted">{short(r.cid, 20)}</div></td>
                <td>{RECORD_TYPES[r.recordType]}</td>
                <td>{nameOf(r.author)}</td>
                <td>{r.org}</td>
                <td>{formatDate(r.createdAt)}</td>
                {r.invalidated
                  ? <td colSpan={2}><Badge tone="red">eliminado</Badge></td>
                  : (
                    <>
                      <td><input value={reasons[r.cid] || ''} placeholder="Motivo da eliminação" onChange={(e) => setReasons({ ...reasons, [r.cid]: e.target.value })} /></td>
                      <td><button className="danger" onClick={() => invalidate(r)} disabled={action.busy || !seed || !reasons[r.cid]}>Eliminar</button></td>
                    </>
                  )}
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <Outcome state={action.state} />
    </Card>
  )
}

const EMERGENCY_TONE: Record<EmergencyStatus, Tone> = { ativa: 'amber', utilizada: 'blue', expirada: 'neutral' }

function Emergencies({ api, patientKey, directory, version }: Context) {
  const [list, setList] = useState<EmergencyAsset[]>([])
  const nameOf = names(directory)
  useEffect(() => { api.emergencies(patientKey).then(setList).catch(() => setList([])) }, [api, patientKey, version])
  return (
    <Card title="Acessos extraordinários" subtitle="Acessos emergenciais aos seus prontuários, para revisão posterior (RF-06).">
      {list.length === 0 ? <p className="muted">Nenhum acesso extraordinário.</p> : (
        <table>
          <thead><tr><th>Solicitante</th><th>Instituição</th><th>Justificativa</th><th>Solicitado em</th><th>Utilizado em</th><th>Situação</th></tr></thead>
          <tbody>
            {list.map((e) => (
              <tr key={e['@key']}>
                <td>{nameOf(e.professional)}</td>
                <td>{nameOf(e.institution)}</td>
                <td>{e.reason}</td>
                <td>{formatDate(e.requestedAt)}</td>
                <td>{formatDate(e.usedAt)}</td>
                <td><Badge tone={EMERGENCY_TONE[e.status]}>{e.status}</Badge></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Card>
  )
}
