import type { AccessLevel, AssetRef, Directory, Operation, ProfessionalType, RecordType } from './types.ts'

export const PROFESSIONAL_TYPES: Record<ProfessionalType, string> = {
  medico: 'Médico',
  enfermeiro: 'Enfermeiro',
  farmaceutico: 'Farmacêutico',
  outro: 'Outro',
}

export const RECORD_TYPES: Record<RecordType, string> = {
  consulta: 'Consulta',
  exame: 'Exame',
  internacao: 'Internação',
  prescricao: 'Prescrição',
  laudo: 'Laudo',
  outro: 'Outro',
}

export const ACCESS_LEVELS: Record<AccessLevel, string> = {
  leitura: 'Leitura',
  leituraEscrita: 'Leitura e escrita',
}

export const OPERATIONS: Record<Operation, string> = {
  CRIACAO: 'Criação',
  LEITURA: 'Leitura',
  LEITURA_EMERGENCIAL: 'Leitura emergencial',
  SOLICITACAO_EMERGENCIAL: 'Solicitação emergencial',
  CONCESSAO: 'Concessão',
  REVOGACAO: 'Revogação',
  INVALIDACAO: 'Invalidação',
  ACESSO_NEGADO: 'Acesso negado',
}

export interface InstitutionForm {
  id: string
  name: string
  cnpj: string
}

// Instituições sugeridas para cada organização.
export const SUGGESTED_INSTITUTIONS: Record<string, InstitutionForm> = {
  Org1: { id: '2345678', name: 'Hospital A', cnpj: '11222333000181' },
  Org2: { id: '3456789', name: 'Hospital B', cnpj: '45997418000153' },
  Org3: { id: '4567890', name: 'Clínica C', cnpj: '06990590000123' },
}

/** Rótulos tipados para uso em <select>. */
export const entries = <K extends string>(m: Record<K, string>) => Object.entries(m) as [K, string][]

// randomCPF gera um CPF com dígitos verificadores válidos, para a demonstração.
export function randomCPF(): string {
  const d = Array.from({ length: 9 }, () => Math.floor(Math.random() * 10))
  for (const n of [9, 10]) {
    const s = d.slice(0, n).reduce((acc, v, i) => acc + v * (n + 1 - i), 0)
    const r = 11 - (s % 11)
    d.push(r > 9 ? 0 : r)
  }
  return d.join('')
}

export function formatDate(v?: string): string {
  if (!v) return '—'
  const d = new Date(v)
  return isNaN(d.getTime()) ? v : d.toLocaleString('pt-BR')
}

export const short = (s: string | undefined, n = 10): string =>
  s && s.length > n + 3 ? s.slice(0, n) + '…' : s || '—'

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// names resolve chaves do ledger para nomes do diretório da API.
export function names(directory: Directory | null) {
  const map: Record<string, string> = {}
  for (const o of directory?.orgs || []) {
    if (o.institution) map[o.institution.key] = o.institution.name
  }
  for (const p of directory?.professionals || []) map[p.key] = p.name
  for (const p of directory?.patients || []) map[p.key] = p.name
  return (ref: string | AssetRef | undefined): string => {
    const key = typeof ref === 'string' ? ref : ref?.['@key']
    return (key && map[key]) || short(key, 18)
  }
}
