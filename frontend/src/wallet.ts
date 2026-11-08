// Carteira simulada do paciente. A chave privada Ed25519 é gerada e mantida
// apenas neste navegador; a API recebe somente a chave pública e as
// assinaturas, e não consegue forjar um consentimento.
import nacl from 'tweetnacl'

const DOMAIN = 'prontuarios-cc/v1'
const PREFIX = 'carteira:'

export type Json = string | number | boolean | null | Json[] | { [key: string]: Json }

export interface KeyPair {
  seed: string
  publicKey: string
}

const toB64 = (bytes: Uint8Array) => btoa(String.fromCharCode(...bytes))
const fromB64 = (s: string) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0))

// canonicalJSON reproduz txdefs.SignedMessage: chaves em ordem alfabética,
// sem espaços e sem escape de HTML.
export function canonicalJSON(value: Json): string {
  if (Array.isArray(value)) return '[' + value.map(canonicalJSON).join(',') + ']'
  if (value !== null && typeof value === 'object') {
    return '{' + Object.keys(value).sort()
      .map((k) => JSON.stringify(k) + ':' + canonicalJSON(value[k]))
      .join(',') + '}'
  }
  return JSON.stringify(value)
}

export function signedMessage(action: string, patientId: string, nonce: number, payload: Record<string, Json>): string {
  return canonicalJSON({ domain: DOMAIN, action, patient: patientId, nonce, ...payload })
}

// formatTime reproduz o formato gravado pelo chaincode (RFC 3339 em UTC, sem
// frações de segundo nulas).
export function formatTime(date: Date): string {
  return date.toISOString().replace(/\.000Z$/, 'Z')
}

export function generateKeyPair(): KeyPair {
  const seed = nacl.randomBytes(32)
  const pair = nacl.sign.keyPair.fromSeed(seed)
  return { seed: toB64(seed), publicKey: toB64(pair.publicKey) }
}

export function sign(seedB64: string, message: string): string {
  const pair = nacl.sign.keyPair.fromSeed(fromB64(seedB64))
  return toB64(nacl.sign.detached(new TextEncoder().encode(message), pair.secretKey))
}

function storage(): Storage | undefined {
  try {
    return globalThis.localStorage
  } catch {
    return undefined
  }
}

export function saveWallet(patientId: string, seed: string): void {
  storage()?.setItem(PREFIX + patientId, seed)
}

export function loadWallet(patientId: string): string | null {
  return storage()?.getItem(PREFIX + patientId) || null
}
