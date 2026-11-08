import { useEffect, useState, type FormEvent } from 'react'
import type { ViewProps } from '../App.tsx'
import type { Api, ProfessionalForm } from '../api.ts'
import { Badge, Card, Field, JsonView, Outcome, useAction } from '../components/ui.tsx'
import type {
  Directory, DirectoryInstitution, DirectoryProfessional, InstitutionAsset, PatientAsset,
  ProfessionalAsset, ProfessionalType,
} from '../types.ts'
import { PROFESSIONAL_TYPES, SUGGESTED_INSTITUTIONS, entries, randomCPF, type InstitutionForm } from '../util.ts'
import { generateKeyPair, saveWallet, type KeyPair } from '../wallet.ts'

export default function Institution({ api, org, institution, directory, refresh }: ViewProps) {
  return (
    <>
      <InstitutionCard api={api} org={org} institution={institution} refresh={refresh} />
      {institution && (
        <>
          <Professionals api={api} org={org} directory={directory} refresh={refresh} />
          <PatientRegistration api={api} refresh={refresh} />
          <PatientLookup api={api} directory={directory} />
        </>
      )}
    </>
  )
}

const emptyInstitution = (org: string): InstitutionForm => SUGGESTED_INSTITUTIONS[org] || { id: '', name: '', cnpj: '' }

function InstitutionCard({ api, org, institution, refresh }: {
  api: Api
  org: string
  institution?: DirectoryInstitution
  refresh: () => Promise<void>
}) {
  const [form, setForm] = useState(emptyInstitution(org))
  const [asset, setAsset] = useState<InstitutionAsset | null>(null)
  const action = useAction()

  useEffect(() => { setForm(emptyInstitution(org)) }, [org])
  useEffect(() => {
    setAsset(null)
    if (institution) api.asset<InstitutionAsset>(institution.key).then(setAsset).catch(() => {})
  }, [api, institution])

  async function submit(e: FormEvent) {
    e.preventDefault()
    const res = await action.run(() => api.registerInstitution(form), 'Instituição cadastrada (RF-01).')
    if (res) refresh()
  }

  async function toggle() {
    if (!asset || !institution) return
    const res = await action.run(() => api.setInstitutionStatus(!asset.active),
      asset.active ? 'Instituição desativada: suas transações de negócio passam a ser recusadas.' : 'Instituição reativada.')
    if (res) setAsset(await api.asset<InstitutionAsset>(institution.key))
  }

  if (institution) {
    return (
      <Card
        title={institution.name}
        subtitle={`CNES ${institution.id} · organização ${org}`}
        actions={asset && <button className="secondary" onClick={toggle} disabled={action.busy}>{asset.active ? 'Desativar' : 'Ativar'}</button>}
      >
        {asset && (
          <p>
            Situação: <Badge tone={asset.active ? 'green' : 'red'}>{asset.active ? 'ativa' : 'inativa'}</Badge>{' '}
            <span className="muted">MSP {asset.mspId} · CNPJ {asset.cnpj}</span>
          </p>
        )}
        <Outcome state={action.state} />
      </Card>
    )
  }

  return (
    <Card title="Cadastrar instituição" subtitle={`Cada organização Fabric representa uma única instituição. A transação é assinada pela ${org}.`}>
      <form onSubmit={submit} className="grid">
        <Field label="CNES"><input value={form.id} onChange={(e) => setForm({ ...form, id: e.target.value })} required /></Field>
        <Field label="Nome"><input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></Field>
        <Field label="CNPJ"><input value={form.cnpj} onChange={(e) => setForm({ ...form, cnpj: e.target.value })} required /></Field>
        <div className="form-actions"><button disabled={action.busy}>Cadastrar</button></div>
      </form>
      <Outcome state={action.state} />
    </Card>
  )
}

const emptyProfessional = (): Required<ProfessionalForm> => ({
  cpf: randomCPF(), name: '', professionalType: 'medico', councilRegistry: '', emergencyAccessAllowed: true,
})

function Professionals({ api, org, directory, refresh }: {
  api: Api
  org: string
  directory: Directory | null
  refresh: () => Promise<void>
}) {
  const [form, setForm] = useState(emptyProfessional)
  const [assets, setAssets] = useState<Record<string, ProfessionalAsset | null>>({})
  const action = useAction()
  const list = (directory?.professionals || []).filter((p) => p.org === org)

  async function load() {
    const loaded = await Promise.all(list.map((p) =>
      api.professional(p.key).then((a) => [p.key, a] as const).catch(() => [p.key, null] as const)))
    setAssets(Object.fromEntries(loaded))
  }
  useEffect(() => { load() }, [directory, org]) // eslint-disable-line react-hooks/exhaustive-deps

  async function submit(e: FormEvent) {
    e.preventDefault()
    const res = await action.run(() => api.registerProfessional(form), 'Profissional cadastrado; o CPF seguiu no mapa transiente e não foi gravado.')
    if (res) {
      setForm(emptyProfessional())
      refresh()
    }
  }

  async function update(p: DirectoryProfessional, change: { active?: boolean; emergencyAccessAllowed?: boolean }) {
    const a = assets[p.key]
    if (!a) return
    const res = await action.run(() => api.setProfessionalStatus(p.key, {
      active: a.active, emergencyAccessAllowed: a.emergencyAccessAllowed, ...change,
    }), 'Situação do profissional atualizada.')
    if (res) load()
  }

  return (
    <Card title="Profissionais de saúde" subtitle="Vinculados à instituição em uso (RF-02).">
      {list.length > 0 && (
        <table>
          <thead><tr><th>Nome</th><th>Categoria</th><th>Registro</th><th>Situação</th><th>Acesso extraordinário</th></tr></thead>
          <tbody>
            {list.map((p) => {
              const a = assets[p.key]
              return (
                <tr key={p.key}>
                  <td>{p.name}</td>
                  <td>{PROFESSIONAL_TYPES[p.professionalType]}</td>
                  <td>{p.councilRegistry}</td>
                  <td>{a && <button className="link" onClick={() => update(p, { active: !a.active })}><Badge tone={a.active ? 'green' : 'red'}>{a.active ? 'ativo' : 'inativo'}</Badge></button>}</td>
                  <td>{a && <button className="link" onClick={() => update(p, { emergencyAccessAllowed: !a.emergencyAccessAllowed })}><Badge tone={a.emergencyAccessAllowed ? 'blue' : 'neutral'}>{a.emergencyAccessAllowed ? 'habilitado' : 'não habilitado'}</Badge></button>}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
      <form onSubmit={submit} className="grid">
        <Field label="Nome"><input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></Field>
        <Field label="CPF" hint="Gerado aleatoriamente para a demonstração">
          <input value={form.cpf} onChange={(e) => setForm({ ...form, cpf: e.target.value })} required />
        </Field>
        <Field label="Categoria">
          <select value={form.professionalType} onChange={(e) => {
            const professionalType = e.target.value as ProfessionalType
            setForm({ ...form, professionalType, emergencyAccessAllowed: professionalType === 'medico' })
          }}>
            {entries(PROFESSIONAL_TYPES).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </select>
        </Field>
        <Field label="Registro no conselho"><input value={form.councilRegistry} placeholder="CRM-DF 12345" onChange={(e) => setForm({ ...form, councilRegistry: e.target.value })} required /></Field>
        <label className="check"><input type="checkbox" checked={form.emergencyAccessAllowed} onChange={(e) => setForm({ ...form, emergencyAccessAllowed: e.target.checked })} /> Habilitado para acesso extraordinário</label>
        <div className="form-actions"><button disabled={action.busy}>Cadastrar profissional</button></div>
      </form>
      <Outcome state={action.state} />
    </Card>
  )
}

function PatientRegistration({ api, refresh }: { api: Api; refresh: () => Promise<void> }) {
  const [form, setForm] = useState({ name: '', cpf: randomCPF() })
  const [wallet, setWallet] = useState<KeyPair | null>(null)
  const action = useAction()

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!wallet) return
    const res = await action.run(
      () => api.registerPatient({ ...form, signingPublicKey: wallet.publicKey }),
      (r) => `Paciente cadastrado com o identificador pseudônimo ${r.patient.id}.`,
    )
    if (res) {
      saveWallet(res.patient.id, wallet.seed)
      setWallet(null)
      setForm({ name: '', cpf: randomCPF() })
      refresh()
    }
  }

  return (
    <Card
      title="Cadastrar paciente"
      subtitle="O par de chaves Ed25519 é gerado na carteira do paciente (este navegador). A API recebe apenas a chave pública; o KMS gera a chave de cifração e distribui os shares aos custodiantes."
    >
      <form onSubmit={submit} className="grid">
        <Field label="Nome" hint="Mantido fora da blockchain"><input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required /></Field>
        <Field label="CPF" hint="Enviado no mapa transiente; o ledger guarda apenas o HMAC">
          <input value={form.cpf} onChange={(e) => setForm({ ...form, cpf: e.target.value })} required />
        </Field>
        <Field label="Carteira do paciente">
          {wallet
            ? <code className="mono small">{wallet.publicKey}</code>
            : <button type="button" className="secondary" onClick={() => setWallet(generateKeyPair())}>Gerar par de chaves</button>}
        </Field>
        <div className="form-actions"><button disabled={action.busy || !wallet}>Cadastrar paciente</button></div>
      </form>
      <Outcome state={action.state} />
    </Card>
  )
}

function PatientLookup({ api, directory }: { api: Api; directory: Directory | null }) {
  const [cpf, setCpf] = useState('')
  const [found, setFound] = useState<PatientAsset | null>(null)
  const action = useAction()

  async function submit(e: FormEvent) {
    e.preventDefault()
    setFound(null)
    const res = await action.run(() => api.lookupPatient(cpf), 'Paciente localizado pelo HMAC do CPF.')
    if (res) setFound(res)
  }

  const name = directory?.patients.find((p) => p.key === found?.['@key'])?.name
  return (
    <Card title="Localizar paciente por CPF" subtitle="Consulta getPatientByCpf: o CPF segue no mapa transiente e o chaincode recalcula o HMAC.">
      <form onSubmit={submit} className="row">
        <input value={cpf} onChange={(e) => setCpf(e.target.value)} placeholder="CPF" required />
        <button disabled={action.busy}>Consultar</button>
      </form>
      <Outcome state={action.state} />
      {found && (
        <>
          {name && <p>Nome no diretório da API: <strong>{name}</strong></p>}
          <JsonView value={found} />
        </>
      )}
    </Card>
  )
}
