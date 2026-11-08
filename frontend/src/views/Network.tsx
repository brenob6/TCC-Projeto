import { useEffect, useState } from 'react'
import type { ViewProps } from '../App.tsx'
import { Badge, Card } from '../components/ui.tsx'
import type { Health } from '../types.ts'
import { errorMessage } from '../util.ts'

const ok = (s: string) => s.startsWith('ok') || s.startsWith('ativo')

export default function Network({ api }: ViewProps) {
  const [health, setHealth] = useState<Health | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function check() {
    setBusy(true)
    try {
      setHealth(await api.health())
      setError(null)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  useEffect(() => { check() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <>
      <Card
        title="Arquitetura"
        subtitle="Cada instituição é uma organização Fabric com um peer e um nó IPFS próprios. A API Gateway assina as transações com a identidade da instituição selecionada no topo."
      >
        <div className="layers">
          <div><Badge tone="blue">Blockchain</Badge><p>Cadastros, consentimentos assinados pelo paciente e trilha de auditoria. Nenhum dado pessoal ou clínico.</p></div>
          <div><Badge tone="amber">IPFS</Badge><p>Envelopes cifrados dos prontuários, endereçados pelo CID registrado no ledger.</p></div>
          <div><Badge tone="green">KMS</Badge><p>Chave de cada paciente dividida por Shamir entre custodiantes; a chave privada nunca é reconstituída.</p></div>
        </div>
      </Card>

      <Card title="Disponibilidade" actions={<button onClick={check} disabled={busy}>{busy ? 'Verificando…' : 'Verificar'}</button>}>
        {error && <p className="error">{error}</p>}
        {health && (
          <>
            <table>
              <thead><tr><th>Organização</th><th>Peer / chaincode</th><th>Nó IPFS</th></tr></thead>
              <tbody>
                {health.orgs.map((o) => (
                  <tr key={o.name}>
                    <td>{o.name}<div className="muted small">{o.mspId}</div></td>
                    <td><Badge tone={ok(o.fabric) ? 'green' : 'red'}>{ok(o.fabric) ? 'disponível' : 'falha'}</Badge> <span className="small muted">{o.fabric}</span></td>
                    <td><Badge tone={ok(o.ipfs) ? 'green' : 'red'}>{ok(o.ipfs) ? 'disponível' : 'falha'}</Badge> <span className="small muted mono">{o.ipfs}</span></td>
                  </tr>
                ))}
              </tbody>
            </table>
            <h3>Custodiantes de chaves (limiar {health.threshold}-de-{health.total})</h3>
            <table>
              <tbody>
                {health.custodians.map((c) => (
                  <tr key={c.address}>
                    <td className="mono">{c.address}</td>
                    <td><Badge tone={ok(c.status) ? 'green' : 'red'}>{ok(c.status) ? 'ativo' : 'indisponível'}</Badge> <span className="small muted">{c.status}</span></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
      </Card>
    </>
  )
}
