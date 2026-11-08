// Package kmsclient expõe o KMS e o envelope cifrado para módulos externos,
// como a API Gateway (../api-gateway). Os pacotes em internal/ não podem ser
// importados fora deste módulo; este pacote apenas os reexporta.
package kmsclient

import (
	"poc-key-sharing/internal/crypto"
	"poc-key-sharing/internal/kms"
)

type (
	// KMS coordena os custodiantes de chaves.
	KMS = kms.KMS
	// Config define o esquema de limiar e os custodiantes disponíveis.
	Config = kms.Config
	// AuditInfo identifica a origem de uma solicitação de decifração.
	AuditInfo = kms.AuditInfo
	// EncryptedRecord é o envelope armazenado na rede IPFS.
	EncryptedRecord = crypto.EncryptedRecord
)

// New abre as conexões gRPC com os custodiantes.
func New(cfg Config) (*KMS, error) { return kms.New(cfg) }
