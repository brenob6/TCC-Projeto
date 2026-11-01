package assettypes

import (
	"github.com/hyperledger-labs/cc-tools/assets"
)

// Professional representa um profissional de saúde vinculado a uma
// instituição (RF-02).
var Professional = assets.AssetType{
	Tag:         "professional",
	Label:       "Profissional de Saúde",
	Description: "Profissional vinculado a uma instituição de saúde",

	Props: []assets.AssetProp{
		{
			Tag:         "id",
			Label:       "Identificador",
			Description: "UUID derivado do hash do CPF e da instituição de vínculo",
			DataType:    "string",
			IsKey:       true,
			Required:    true,
		},
		{
			Tag:      "cpfHash",
			Label:    "Hash do CPF",
			DataType: "sha256",
			Required: true,
		},
		{
			Tag:      "name",
			Label:    "Nome",
			DataType: "string",
			Required: true,
		},
		{
			Tag:      "professionalType",
			Label:    "Categoria",
			DataType: "professionalType",
			Required: true,
		},
		{
			Tag:         "councilRegistry",
			Label:       "Registro no conselho",
			Description: "Ex.: CRM-DF 12345",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:      "institution",
			Label:    "Instituição",
			DataType: "->institution",
			Required: true,
		},
		{
			Tag:         "emergencyAccessAllowed",
			Label:       "Acesso extraordinário",
			Description: "Indica se o profissional pode solicitar acesso emergencial",
			DataType:    "boolean",
			Required:    true,
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
