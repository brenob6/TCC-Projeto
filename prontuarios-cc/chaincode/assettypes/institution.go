// Package assettypes define os ativos registrados no ledger do canal
// prontuarios. Nenhum deles contém dado clínico ou dado pessoal em claro.
package assettypes

import (
	"github.com/hyperledger-labs/cc-tools/assets"
)

// Institution representa uma instituição de saúde participante (RF-01).
var Institution = assets.AssetType{
	Tag:         "institution",
	Label:       "Instituição de Saúde",
	Description: "Instituição participante da rede, vinculada a uma organização Fabric",

	Props: []assets.AssetProp{
		{
			Tag:         "id",
			Label:       "CNES",
			Description: "Código CNES da instituição",
			DataType:    "string",
			IsKey:       true,
			Required:    true,
		},
		{
			Tag:      "name",
			Label:    "Nome",
			DataType: "string",
			Required: true,
		},
		{
			Tag:      "cnpj",
			Label:    "CNPJ",
			DataType: "cnpj",
			Required: true,
		},
		{
			Tag:         "mspId",
			Label:       "MSP",
			Description: "Organização Fabric que representa a instituição",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:      "active",
			Label:    "Ativa",
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
