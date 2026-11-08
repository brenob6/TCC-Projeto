// Cliente da API Gateway. O cabeçalho X-Org indica a instituição em nome da
// qual a API assina as transações.
import type {
  AccessibleRecord, AccessLogAsset, AccessResult, Asset, Directory, EmergencyAsset, Health,
  MedicalDocument, PatientAsset, PatientRecord, PermissionAsset, PermissionCheck,
  InstitutionAsset, ProfessionalAsset, ProfessionalType, Signature, TraceStep, TxResponse,
} from './types.ts'

export class ApiError extends Error {
  readonly status: number
  readonly trace?: TraceStep[]

  constructor(message: string, status: number, trace?: TraceStep[]) {
    super(message)
    this.status = status
    this.trace = trace
  }
}

async function request<T>(org: string, method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch('/api' + path, {
    method,
    headers: { 'Content-Type': 'application/json', 'X-Org': org },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new ApiError(data.error || `${res.status} ${res.statusText}`, res.status, data.trace)
  }
  return data as T
}

const enc = encodeURIComponent

export interface ProfessionalForm {
  cpf: string
  name: string
  professionalType: ProfessionalType
  councilRegistry: string
  emergencyAccessAllowed?: boolean
}

export interface GrantForm extends Signature {
  professional?: string
  institution?: string
  accessLevel: string
  purpose: string
  records: string[]
  expiresAt: string
}

export interface RecordForm extends Omit<MedicalDocument, 'file'> {
  patient: string
  author: string
  file?: MedicalDocument['file']
}

export function api(org: string) {
  const get = <T>(p: string) => request<T>(org, 'GET', p)
  const post = <T>(p: string, b: unknown = {}) => request<T>(org, 'POST', p, b)
  const put = <T>(p: string, b: unknown = {}) => request<T>(org, 'PUT', p, b)
  return {
    health: () => get<Health>('/health'),
    directory: () => get<Directory>('/directory'),
    asset: <T extends Asset>(key: string) => get<T>(`/assets/${enc(key)}`),

    registerInstitution: (b: { id: string; name: string; cnpj: string }) =>
      post<TxResponse & { institution: InstitutionAsset }>('/institutions', b),
    setInstitutionStatus: (active: boolean) => put<TxResponse>('/institutions/status', { active }),

    registerProfessional: (b: ProfessionalForm) =>
      post<TxResponse & { professional: ProfessionalAsset }>('/professionals', b),
    professional: (key: string) => get<ProfessionalAsset>(`/professionals/${enc(key)}`),
    setProfessionalStatus: (key: string, b: { active: boolean; emergencyAccessAllowed?: boolean }) =>
      put<TxResponse>(`/professionals/${enc(key)}/status`, b),

    registerPatient: (b: { cpf: string; name: string; signingPublicKey: string }) =>
      post<TxResponse & { patient: PatientAsset }>('/patients', b),
    lookupPatient: (cpf: string) => post<PatientAsset>('/patients/lookup', { cpf }),
    patient: (key: string) => get<PatientAsset>(`/patients/${enc(key)}`),
    records: (key: string, prof: string) =>
      get<AccessibleRecord[]>(`/patients/${enc(key)}/records?professional=${enc(prof)}`),
    allRecords: (key: string) => get<PatientRecord[]>(`/patients/${enc(key)}/all-records`),
    permissions: (key: string) => get<PermissionAsset[]>(`/patients/${enc(key)}/permissions`),
    grant: (key: string, b: GrantForm) => post<TxResponse>(`/patients/${enc(key)}/permissions`, b),
    check: (key: string, prof: string) =>
      get<PermissionCheck>(`/patients/${enc(key)}/check?professional=${enc(prof)}`),
    history: (key: string, q: Record<string, string> = {}) =>
      get<AccessLogAsset[]>(`/patients/${enc(key)}/history?${new URLSearchParams(q)}`),
    emergencies: (key: string) => get<EmergencyAsset[]>(`/patients/${enc(key)}/emergencies`),

    revoke: (key: string, b: Signature) => post<TxResponse>(`/permissions/${enc(key)}/revoke`, b),

    addRecord: (b: RecordForm) => post<TxResponse & { cid: string }>('/records', b),
    access: (cid: string, professional: string) =>
      post<AccessResult>(`/records/${enc(cid)}/access`, { professional }),
    emergency: (cid: string, professional: string, reason: string) =>
      post<AccessResult>(`/records/${enc(cid)}/emergency`, { professional, reason }),
    invalidate: (cid: string, b: Signature & { reason: string }) =>
      post<TxResponse>(`/records/${enc(cid)}/invalidate`, b),
  }
}

export type Api = ReturnType<typeof api>
