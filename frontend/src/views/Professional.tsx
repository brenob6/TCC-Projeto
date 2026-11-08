import { useEffect, useState, type ChangeEvent, type FormEvent } from 'react'
import type { ViewProps } from '../App.tsx'
import type { Api, RecordForm } from '../api.ts'
import { Badge, Card, Field, Outcome, useAction } from '../components/ui.tsx'
import type {
  AccessibleRecord, AccessResult, Directory, MedicalDocument, PatientRecord, PermissionCheck, RecordType,
} from '../types.ts'
import { OPERATIONS, RECORD_TYPES, entries, errorMessage, formatDate, names, short } from '../util.ts'

/** Contexto compartilhado pelos painéis do atendimento. */
interface Context {
  api: Api
  profKey: string
  patientKey: string
  directory: Directory | null
  version: number
  bump: () => void
}

export default function Professional({ api, org, institution, directory }: ViewProps) {
  const professionals = (directory?.professionals || []).filter((p) => p.org === org)
  const patients = directory?.patients || []
  const [profKey, setProfKey] = useState('')
  const [patientKey, setPatientKey] = useState('')
  const [version, setVersion] = useState(0)
  const bump = () => setVersion((v) => v + 1)

  useEffect(() => {
    if (!professionals.some((p) => p.key === profKey)) setProfKey(professionals[0]?.key || '')
  }, [org, directory]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (!patientKey && patients[0]) setPatientKey(patients[0].key) }, [directory]) // eslint-disable-line react-hooks/exhaustive-deps

  if (!institution) return <Card title="Profissional"><p className="muted">Cadastre a instituição da {org} na aba Instituição.</p></Card>

  const ctx: Context = { api, profKey, patientKey, directory, version, bump }
  return (
    <>
      <Card title="Atendimento" subtitle={`Ações executadas por um profissional da ${institution.name}. A API valida no chaincode que o profissional pertence à instituição que assina a transação.`}>
        <div className="grid">
          <Field label="Profissional">
            <select value={profKey} onChange={(e) => setProfKey(e.target.value)}>
              {professionals.length === 0 && <option value="">Nenhum profissional cadastrado</option>}
              {professionals.map((p) => <option key={p.key} value={p.key}>{p.name} — {p.councilRegistry}</option>)}
            </select>
          </Field>
          <Field label="Paciente">
            <select value={patientKey} onChange={(e) => setPatientKey(e.target.value)}>
              {patients.length === 0 && <option value="">Nenhum paciente cadastrado</option>}
              {patients.map((p) => <option key={p.key} value={p.key}>{p.name} — {short(p.id, 8)}</option>)}
            </select>
          </Field>
        </div>
        {profKey && patientKey && <PermissionStatus {...ctx} />}
      </Card>
      {profKey && patientKey && (
        <>
          <Records {...ctx} />
          <NewRecord {...ctx} />
        </>
      )}
    </>
  )
}

function PermissionStatus({ api, profKey, patientKey, version }: Context) {
  const [perm, setPerm] = useState<PermissionCheck | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    setPerm(null)
    api.check(patientKey, profKey).then((p) => { setPerm(p); setError(null) }).catch((e) => setError(errorMessage(e)))
  }, [api, profKey, patientKey, version])
  if (error) return <p className="error">{error}</p>
  if (!perm) return null
  return (
    <p>
      Consentimento vigente (checkPermission, sem auditoria):{' '}
      <Badge tone={perm.read ? 'green' : 'red'}>{perm.read ? 'leitura permitida' : 'sem leitura'}</Badge>{' '}
      <Badge tone={perm.write ? 'green' : 'red'}>{perm.write ? 'escrita permitida' : 'sem escrita'}</Badge>
    </p>
  )
}

function Records({ api, profKey, patientKey, directory, version, bump }: Context) {
  const [accessible, setAccessible] = useState<AccessibleRecord[]>([])
  const [all, setAll] = useState<PatientRecord[]>([])
  const [reasons, setReasons] = useState<Record<string, string>>({})
  const [opened, setOpened] = useState<AccessResult | null>(null)
  const action = useAction()
  const nameOf = names(directory)

  useEffect(() => {
    api.records(patientKey, profKey).then(setAccessible).catch(() => setAccessible([]))
    api.allRecords(patientKey).then(setAll).catch(() => setAll([]))
  }, [api, profKey, patientKey, version])

  const titles = Object.fromEntries(all.map((r) => [r.cid, r.title]))
  const active = all.filter((r) => !r.invalidated)

  async function open(cid: string, emergency: boolean) {
    setOpened(null)
    const res = await action.run(
      () => (emergency ? api.emergency(cid, profKey, reasons[cid] || '') : api.access(cid, profKey)),
      (r) => r.granted
        ? `${OPERATIONS[r.operation]} autorizada e registrada na trilha de auditoria.`
        : `Acesso negado e registrado na trilha de auditoria: ${r.reason}.`,
    )
    if (res) {
      setOpened(res.granted ? res : null)
      if (!res.granted) action.setState((s) => s && { ...s, error: s.message, message: undefined })
    }
    bump()
  }

  return (
    <>
      <Card title="Prontuários acessíveis" subtitle="listPatientRecords: somente os prontuários cobertos por consentimento ou por acesso extraordinário ativo. A leitura passa por requestRecordAccess (Figura 8).">
        {accessible.length === 0 ? <p className="muted">Nenhum prontuário acessível a este profissional.</p> : (
          <table>
            <thead><tr><th>Título</th><th>Tipo</th><th>Autor</th><th>Registrado em</th><th></th></tr></thead>
            <tbody>
              {accessible.map((r) => (
                <tr key={r.cid}>
                  <td>{titles[r.cid] || <span className="mono">{short(r.cid, 16)}</span>} {r.viaEmergency && <Badge tone="amber">emergencial</Badge>}</td>
                  <td>{RECORD_TYPES[r.recordType]}</td>
                  <td>{nameOf(r.author)}</td>
                  <td>{formatDate(r.createdAt)}</td>
                  <td><button onClick={() => open(r.cid, false)} disabled={action.busy}>Abrir</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <Card title="Recuperação extraordinária" subtitle="Metadados mantidos pela API para todos os prontuários do paciente. Sem consentimento, a leitura é negada e auditada; com justificativa, o médico habilitado obtém um acesso de uso único válido por 1 hora (Figura 9).">
        {active.length === 0 ? <p className="muted">Nenhum prontuário registrado para o paciente.</p> : (
          <table>
            <thead><tr><th>Título</th><th>Origem</th><th>Justificativa</th><th></th></tr></thead>
            <tbody>
              {active.map((r) => (
                <tr key={r.cid}>
                  <td>{r.title}<div className="mono small muted">{short(r.cid, 20)}</div></td>
                  <td>{r.org}</td>
                  <td><input value={reasons[r.cid] || ''} placeholder="Ex.: paciente inconsciente no pronto-socorro" onChange={(e) => setReasons({ ...reasons, [r.cid]: e.target.value })} /></td>
                  <td className="row">
                    <button className="secondary" onClick={() => open(r.cid, false)} disabled={action.busy}>Solicitar leitura</button>
                    <button className="danger" onClick={() => open(r.cid, true)} disabled={action.busy || (reasons[r.cid] || '').trim().length < 10}>Acesso emergencial</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {(action.state || opened) && (
        <Card title="Resultado da leitura">
          <Outcome state={action.state} />
          {opened?.document && <DocumentView doc={opened.document} />}
        </Card>
      )}
    </>
  )
}

function DocumentView({ doc }: { doc: MedicalDocument }) {
  return (
    <article className="document">
      <header>
        <Badge tone="blue">{RECORD_TYPES[doc.recordType] || doc.recordType}</Badge>
        <h3>{doc.title}</h3>
      </header>
      {doc.content && <p className="pre">{doc.content}</p>}
      {doc.file && (
        <p>
          Anexo: <a download={doc.file.name} href={`data:${doc.file.type || 'application/octet-stream'};base64,${doc.file.data}`}>{doc.file.name}</a>
        </p>
      )}
    </article>
  )
}

const MAX_FILE_BASE64 = 1_400_000 // cerca de 1 MB

type NewRecordForm = Pick<RecordForm, 'recordType' | 'title' | 'content'>
const emptyRecord: NewRecordForm = { recordType: 'consulta', title: '', content: '' }

function NewRecord({ api, profKey, patientKey, bump }: Context) {
  const [form, setForm] = useState<NewRecordForm>(emptyRecord)
  const [file, setFile] = useState<MedicalDocument['file'] | null>(null)
  const action = useAction()

  async function pick(e: ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0]
    if (!f) return setFile(null)
    const data = await new Promise<string>((resolve) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result).split(',')[1])
      reader.readAsDataURL(f)
    })
    setFile({ name: f.name, type: f.type, data })
  }

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const target = e.currentTarget
    const res = await action.run(
      () => api.addRecord({ ...form, file: file || undefined, patient: patientKey, author: profKey }),
      (r) => `Prontuário cifrado, armazenado no IPFS e registrado na blockchain com o CID ${r.cid}.`,
    )
    if (res) {
      setForm(emptyRecord)
      setFile(null)
      target.reset()
      bump()
    }
  }

  return (
    <Card title="Adicionar prontuário" subtitle="Figura 7: a API cifra o documento com a chave pública do paciente, grava o envelope no IPFS e registra o CID e o SHA-256 no ledger. Exige consentimento de leitura e escrita.">
      <form onSubmit={submit} className="grid">
        <Field label="Tipo">
          <select value={form.recordType} onChange={(e) => setForm({ ...form, recordType: e.target.value as RecordType })}>
            {entries(RECORD_TYPES).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </select>
        </Field>
        <Field label="Título"><input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} required /></Field>
        <Field label="Conteúdo"><textarea rows={4} value={form.content} onChange={(e) => setForm({ ...form, content: e.target.value })} /></Field>
        <Field label="Anexo (opcional)" hint="Até 1 MB"><input type="file" onChange={pick} /></Field>
        <div className="form-actions"><button disabled={action.busy || (file?.data.length ?? 0) > MAX_FILE_BASE64}>Registrar prontuário</button></div>
      </form>
      <Outcome state={action.state} />
    </Card>
  )
}
