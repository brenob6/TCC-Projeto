package assettypes

import (
	"github.com/hyperledger-labs/cc-tools/assets"
)

// AccessPermission registra o consentimento do paciente para que um
// profissional ou uma instituição acesse seus prontuários (RF-04, RF-05).
var AccessPermission = assets.AssetType{
	Tag:         "accessPermission",
	Label:       "Permissão de Acesso",
	Description: "Consentimento assinado pelo paciente",

	Props: []assets.AssetProp{
		{
			Tag:         "id",
			Label:       "Identificador",
			Description: "ID da transação de concessão",
			DataType:    "string",
			IsKey:       true,
			Required:    true,
		},
		{
			Tag:      "patient",
			Label:    "Paciente",
			DataType: "->patient",
			Required: true,
		},
		{
			Tag:         "professional",
			Label:       "Profissional autorizado",
			Description: "Preenchido quando a permissão é individual",
			DataType:    "->professional",
		},
		{
			Tag:         "institution",
			Label:       "Instituição autorizada",
			Description: "Preenchido quando a permissão vale para todos os profissionais da instituição",
			DataType:    "->institution",
		},
		{
			Tag:      "accessLevel",
			Label:    "Nível de acesso",
			DataType: "accessLevel",
			Required: true,
		},
		{
			Tag:         "records",
			Label:       "Prontuários",
			Description: "Escopo restrito; vazio indica todos os prontuários do paciente",
			DataType:    "[]->medicalRecord",
		},
		{
			Tag:         "purpose",
			Label:       "Finalidade",
			Description: "Finalidade do tratamento declarada ao paciente",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:      "grantedAt",
			Label:    "Concedida em",
			DataType: "datetime",
			Required: true,
		},
		{
			Tag:         "expiresAt",
			Label:       "Válida até",
			Description: "Vazio indica validade até a revogação",
			DataType:    "datetime",
		},
		{
			Tag:      "status",
			Label:    "Situação",
			DataType: "permissionStatus",
			Required: true,
		},
		{
			Tag:      "revokedAt",
			Label:    "Revogada em",
			DataType: "datetime",
		},
		{
			Tag:         "patientSignature",
			Label:       "Assinatura do paciente",
			Description: "Assinatura Ed25519 (Base64) da mensagem de concessão",
			DataType:    "string",
			Required:    true,
		},
	},
}
