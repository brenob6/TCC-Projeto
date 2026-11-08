import { useCallback, useEffect, useMemo, useState, type ComponentType } from 'react'
import { api, type Api } from './api.ts'
import type { Directory, DirectoryInstitution } from './types.ts'
import { errorMessage } from './util.ts'
import Network from './views/Network.tsx'
import Institution from './views/Institution.tsx'
import Professional from './views/Professional.tsx'
import Patient from './views/Patient.tsx'
import Audit from './views/Audit.tsx'

/** Propriedades recebidas por todas as abas. */
export interface ViewProps {
  api: Api
  org: string
  institution?: DirectoryInstitution
  directory: Directory | null
  refresh: () => Promise<void>
}

const TABS: { id: string; label: string; view: ComponentType<ViewProps> }[] = [
  { id: 'rede', label: 'Rede', view: Network },
  { id: 'instituicao', label: 'Instituição', view: Institution },
  { id: 'profissional', label: 'Profissional', view: Professional },
  { id: 'paciente', label: 'Paciente', view: Patient },
  { id: 'auditoria', label: 'Auditoria', view: Audit },
]

export default function App() {
  const [tab, setTab] = useState('rede')
  const [org, setOrg] = useState('Org1')
  const [directory, setDirectory] = useState<Directory | null>(null)
  const [error, setError] = useState<string | null>(null)

  const client = useMemo(() => api(org), [org])

  const refresh = useCallback(async () => {
    try {
      setDirectory(await api('Org1').directory())
      setError(null)
    } catch (err) {
      setError(errorMessage(err))
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  const current = directory?.orgs.find((o) => o.name === org)
  const View = TABS.find((t) => t.id === tab)!.view

  return (
    <div className="app">
      <header className="topbar">
        <div>
          <h1>Prontuários Eletrônicos</h1>
          <p className="muted">Controle de acesso com Hyperledger Fabric, IPFS e KMS de limiar</p>
        </div>
        <label className="org-select">
          <span>Instituição em uso</span>
          <select value={org} onChange={(e) => setOrg(e.target.value)}>
            {(directory?.orgs || [{ name: 'Org1', mspId: 'Org1MSP' }]).map((o) => (
              <option key={o.name} value={o.name}>
                {o.name}{o.institution ? ` — ${o.institution.name}` : ' (sem instituição)'}
              </option>
            ))}
          </select>
        </label>
      </header>

      <nav className="tabs">
        {TABS.map((t) => (
          <button key={t.id} className={t.id === tab ? 'active' : ''} onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}
      </nav>

      {error && <div className="outcome outcome-error"><p>API indisponível: {error}</p></div>}

      <main>
        <View api={client} org={org} institution={current?.institution} directory={directory} refresh={refresh} />
      </main>
    </div>
  )
}
