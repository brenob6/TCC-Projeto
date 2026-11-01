package assettypes

import (
	"github.com/hyperledger-labs/cc-tools/assets"
)

// Patient representa o titular dos prontuários (RF-02). O ativo guarda apenas
// identificadores pseudônimos e chaves públicas; nome e CPF não são gravados.
var Patient = assets.AssetType{
	Tag:         "patient",
	Label:       "Paciente",
	Description: "Titular dos prontuários, identificado de forma pseudônima",

	Props: []assets.AssetProp{
		{
			Tag:         "id",
			Label:       "Identificador",
			Description: "UUID derivado do hash do CPF",
			DataType:    "string",
			IsKey:       true,
			Required:    true,
		},
		{
			Tag:         "cpfHash",
			Label:       "Hash do CPF",
			Description: "HMAC-SHA256 do CPF com o segredo do consórcio",
			DataType:    "sha256",
			Required:    true,
		},
		{
			Tag:         "kmsKeyId",
			Label:       "Chave no KMS",
			Description: "Identificador da chave de cifração do paciente no KMS",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:         "publicKey",
			Label:       "Chave pública de cifração",
			Description: "Chave pública ElGamal usada pelo KMS para cifrar os prontuários",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:         "signingPublicKey",
			Label:       "Chave pública de assinatura",
			Description: "Chave Ed25519 com que o paciente assina concessões, revogações e eliminações",
			DataType:    "ed25519PubKey",
			Required:    true,
		},
		{
			Tag:          "signatureNonce",
			Label:        "Contador de assinaturas",
			Description:  "Último nonce aceito; cada operação assinada deve usar o valor seguinte",
			DataType:     "number",
			Required:     true,
			DefaultValue: 0,
		},
		{
			Tag:      "active",
			Label:    "Ativo",
			DataType: "boolean",
			Required: true,
		},
		{
			Tag:      "registeredAt",
			Label:    "Data de cadastro",
			DataType: "datetime",
			Required: true,
		},
	},
}
