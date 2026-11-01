package assettypes

import (
	"github.com/hyperledger-labs/cc-tools/assets"
)

// AccessLog é a trilha de auditoria das operações sobre prontuários e
// permissões (RF-03, RF-08). Os registros são somente de inserção.
var AccessLog = assets.AssetType{
	Tag:         "accessLog",
	Label:       "Registro de Auditoria",
	Description: "Operação auditada sobre prontuários e permissões",

	Props: []assets.AssetProp{
		{
			Tag:         "id",
			Label:       "Identificador",
			Description: "ID da transação que originou o registro",
			DataType:    "string",
			IsKey:       true,
			Required:    true,
		},
		{
			Tag:      "operation",
			Label:    "Operação",
			DataType: "operationType",
			Required: true,
		},
		{
			Tag:      "patient",
			Label:    "Paciente",
			DataType: "->patient",
			Required: true,
		},
		{
			Tag:      "record",
			Label:    "Prontuário",
			DataType: "->medicalRecord",
		},
		{
			Tag:         "actor",
			Label:       "Responsável",
			Description: "Vazio quando a operação foi assinada pelo próprio paciente",
			DataType:    "->professional",
		},
		{
			Tag:         "institution",
			Label:       "Instituição de origem",
			Description: "Instituição da organização que submeteu a transação",
			DataType:    "->institution",
			Required:    true,
		},
		{
			Tag:      "callerMsp",
			Label:    "MSP do chamador",
			DataType: "string",
			Required: true,
		},
		{
			Tag:      "emergency",
			Label:    "Acesso extraordinário",
			DataType: "->emergencyAccess",
		},
		{
			Tag:      "permission",
			Label:    "Permissão",
			DataType: "->accessPermission",
		},
		{
			Tag:      "details",
			Label:    "Detalhes",
			DataType: "string",
		},
		{
			Tag:      "occurredAt",
			Label:    "Momento",
			DataType: "datetime",
			Required: true,
		},
	},
}
