package assettypes

import (
	"github.com/hyperledger-labs/cc-tools/assets"
)

// MedicalRecord referencia um prontuário cifrado armazenado no IPFS
// (RF-03, RF-07). O conteúdo nunca é gravado no ledger.
var MedicalRecord = assets.AssetType{
	Tag:         "medicalRecord",
	Label:       "Prontuário",
	Description: "Referência a um prontuário cifrado armazenado na rede IPFS",

	Props: []assets.AssetProp{
		{
			Tag:         "cid",
			Label:       "CID",
			Description: "Identificador do envelope cifrado no IPFS",
			DataType:    "string",
			IsKey:       true,
			Required:    true,
		},
		{
			Tag:         "contentHash",
			Label:       "Hash do conteúdo",
			Description: "SHA-256 do envelope cifrado",
			DataType:    "sha256",
			Required:    true,
		},
		{
			Tag:      "patient",
			Label:    "Paciente",
			DataType: "->patient",
			Required: true,
		},
		{
			Tag:      "author",
			Label:    "Autor",
			DataType: "->professional",
			Required: true,
		},
		{
			Tag:      "institution",
			Label:    "Instituição de origem",
			DataType: "->institution",
			Required: true,
		},
		{
			Tag:      "recordType",
			Label:    "Tipo",
			DataType: "recordType",
			Required: true,
		},
		{
			Tag:         "kmsKeyId",
			Label:       "Chave no KMS",
			Description: "Chave usada na cifração",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:      "createdAt",
			Label:    "Data de registro",
			DataType: "datetime",
			Required: true,
		},
		{
			Tag:          "invalidated",
			Label:        "Eliminado",
			Description:  "Eliminação lógica a pedido do titular",
			DataType:     "boolean",
			DefaultValue: false,
		},
	},
}
