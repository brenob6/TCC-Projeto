import { test } from 'node:test'
import assert from 'node:assert/strict'
import nacl from 'tweetnacl'
import { canonicalJSON, formatTime, generateKeyPair, sign, signedMessage } from './wallet.ts'

const fromB64 = (s: string) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0))

test('mensagem de concessão segue o formato do README do chaincode', () => {
  const msg = signedMessage('grantPermission', '<id>', 1, {
    professional: '<id>', institution: '', accessLevel: 'leitura',
    purpose: 'acompanhamento clínico', records: [], expiresAt: '',
  })
  assert.equal(msg,
    '{"accessLevel":"leitura","action":"grantPermission","domain":"prontuarios-cc/v1","expiresAt":"","institution":"","nonce":1,"patient":"<id>","professional":"<id>","purpose":"acompanhamento clínico","records":[]}')
})

test('canonicalJSON não escapa HTML', () => {
  assert.equal(canonicalJSON({ b: '<&>', a: 1 }), '{"a":1,"b":"<&>"}')
})

test('formatTime remove milissegundos nulos', () => {
  assert.equal(formatTime(new Date('2026-12-01T03:00:00.000Z')), '2026-12-01T03:00:00Z')
})

test('assinatura confere com a chave pública', () => {
  const { seed, publicKey } = generateKeyPair()
  const msg = 'mensagem'
  assert.ok(nacl.sign.detached.verify(new TextEncoder().encode(msg), fromB64(sign(seed, msg)), fromB64(publicKey)))
})
