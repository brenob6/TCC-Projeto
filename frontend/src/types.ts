// Tipos das respostas da API Gateway e dos ativos do chaincode.

/** Referência a um ativo gravada pelo cc-tools. */
export interface AssetRef {
  '@assetType': string
  '@key': string
}

export interface Asset {
  '@assetType': string
  '@key': string
  '@lastTx'?: string
}

export interface TraceStep {
  layer: 'API' | 'Blockchain' | 'IPFS' | 'KMS'
  action: string
  detail?: string
  txId?: string
}

/** Campos comuns às respostas de escrita. */
export interface TxResponse {
  txId?: string
  trace?: TraceStep[]
}

// ---- diretório mantido fora da blockchain pela API ----

export interface DirectoryInstitution {
  org: string
  key: string
  id: string
  name: string
}

export interface DirectoryProfessional {
  org: string
  key: string
  id: string
  name: string
  professionalType: ProfessionalType
  councilRegistry: string
}

export interface DirectoryPatient {
  key: string
  id: string
  name: string
  org: string
  kmsKeyId: string
}

export interface OrgEntry {
  name: string
  mspId: string
  institution?: DirectoryInstitution
}

export interface Directory {
  orgs: OrgEntry[]
  professionals: DirectoryProfessional[]
  patients: DirectoryPatient[]
}

// ---- ativos do ledger ----

export type ProfessionalType = 'medico' | 'enfermeiro' | 'farmaceutico' | 'outro'
export type RecordType = 'consulta' | 'exame' | 'internacao' | 'prescricao' | 'laudo' | 'outro'
export type AccessLevel = 'leitura' | 'leituraEscrita'
export type PermissionStatus = 'ativa' | 'revogada' | 'expirada'
export type EmergencyStatus = 'ativa' | 'utilizada' | 'expirada'
export type Operation =
  | 'CRIACAO'
  | 'LEITURA'
  | 'LEITURA_EMERGENCIAL'
  | 'SOLICITACAO_EMERGENCIAL'
  | 'CONCESSAO'
  | 'REVOGACAO'
  | 'INVALIDACAO'
  | 'ACESSO_NEGADO'

export interface InstitutionAsset extends Asset {
  id: string
  name: string
  cnpj: string
  mspId: string
  active: boolean
  registeredAt: string
}

export interface ProfessionalAsset extends Asset {
  id: string
  cpfHash: string
  name: string
  professionalType: ProfessionalType
  councilRegistry: string
  institution: AssetRef
  emergencyAccessAllowed: boolean
  active: boolean
  registeredAt: string
}

export interface PatientAsset extends Asset {
  id: string
  cpfHash: string
  kmsKeyId: string
  publicKey: string
  signingPublicKey: string
  signatureNonce: number
  active: boolean
  registeredAt: string
}

export interface PermissionAsset extends Asset {
  id: string
  patient: AssetRef
  professional?: AssetRef
  institution?: AssetRef
  accessLevel: AccessLevel
  records?: AssetRef[]
  purpose: string
  grantedAt: string
  expiresAt?: string
  status: PermissionStatus
  revokedAt?: string
  patientSignature: string
}

export interface EmergencyAsset extends Asset {
  id: string
  patient: AssetRef
  professional: AssetRef
  institution: AssetRef
  record: AssetRef
  reason: string
  requestedAt: string
  expiresAt: string
  status: EmergencyStatus
  usedAt?: string
}

export interface AccessLogAsset extends Asset {
  id: string
  operation: Operation
  patient: AssetRef
  record?: AssetRef
  actor?: AssetRef
  institution: AssetRef
  callerMsp: string
  emergency?: AssetRef
  permission?: AssetRef
  details?: string
  occurredAt: string
}

// ---- respostas específicas ----

export interface Health {
  orgs: { name: string; mspId: string; fabric: string; ipfs: string }[]
  custodians: { address: string; status: string }[]
  threshold: number
  total: number
}

export interface PermissionCheck {
  read: boolean
  write: boolean
  permission?: string
  emergency?: string
}

/** Prontuário devolvido por listPatientRecords. */
export interface AccessibleRecord {
  cid: string
  recordType: RecordType
  author: string
  institution: string
  createdAt: string
  viaEmergency: boolean
}

/** Prontuário do diretório da API, com a situação lida do ledger. */
export interface PatientRecord {
  cid: string
  key: string
  title: string
  recordType: RecordType
  author: string
  org: string
  createdAt: string
  invalidated?: boolean
  error?: string
}

export interface MedicalDocument {
  title: string
  recordType: RecordType
  content: string
  file?: { name: string; type: string; data: string }
}

export interface AccessResult extends TxResponse {
  granted: boolean
  operation: Operation
  accessLog: string
  reason?: string
  document?: MedicalDocument
  emergencyTxId?: string
}

export interface Signature {
  nonce: number
  signature: string
}
