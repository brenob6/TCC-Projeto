package assettypes

import (
	"github.com/hyperledger-labs/cc-tools/assets"
)

// EmergencyAccess registra uma permissão extraordinária, de uso único e prazo
// curto, concedida sem autorização prévia do paciente (RF-06).
var EmergencyAccess = assets.AssetType{
	Tag:         "emergencyAccess",
	Label:       "Acesso Extraordinário",
	Description: "Permissão emergencial de uso único para um prontuário",

	Props: []assets.AssetProp{
		{
			Tag:         "id",
			Label:       "Identificador",
			Description: "ID da transação de solicitação",
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
			Tag:      "professional",
			Label:    "Solicitante",
			DataType: "->professional",
			Required: true,
		},
		{
			Tag:      "institution",
			Label:    "Instituição",
			DataType: "->institution",
			Required: true,
		},
		{
			Tag:      "record",
			Label:    "Prontuário",
			DataType: "->medicalRecord",
			Required: true,
		},
		{
			Tag:         "reason",
			Label:       "Justificativa",
			Description: "Motivo do acesso extraordinário",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:      "requestedAt",
			Label:    "Solicitado em",
			DataType: "datetime",
			Required: true,
		},
		{
			Tag:      "expiresAt",
			Label:    "Válido até",
			DataType: "datetime",
			Required: true,
		},
		{
			Tag:      "status",
			Label:    "Situação",
			DataType: "emergencyStatus",
			Required: true,
		},
		{
			Tag:      "usedAt",
			Label:    "Utilizado em",
			DataType: "datetime",
		},
	},
}
