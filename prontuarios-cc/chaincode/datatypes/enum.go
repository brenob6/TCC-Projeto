package datatypes

import (
	"fmt"
	"strings"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
)

// stringEnum cria um tipo cujo valor deve pertencer a um conjunto fixo de
// strings. dropDown associa o rótulo exibido na interface ao valor armazenado.
func stringEnum(description string, dropDown map[string]string) assets.DataType {
	values := make(map[string]interface{}, len(dropDown))
	accepted := make(map[string]bool, len(dropDown))
	for label, value := range dropDown {
		values[label] = value
		accepted[value] = true
	}

	return assets.DataType{
		AcceptedFormats: []string{"string"},
		DropDownValues:  values,
		Description:     description,
		Parse: func(data interface{}) (string, interface{}, errors.ICCError) {
			value, ok := data.(string)
			if !ok {
				return "", nil, errors.NewCCError("o valor deve ser uma string", 400)
			}
			if !accepted[value] {
				return "", nil, errors.NewCCError(fmt.Sprintf("valor '%s' inválido (%s)", value, strings.ToLower(description)), 400)
			}
			return value, value, nil
		},
	}
}

// Categorias profissionais.
const (
	ProfessionalMedico       = "medico"
	ProfessionalEnfermeiro   = "enfermeiro"
	ProfessionalFarmaceutico = "farmaceutico"
	ProfessionalOutro        = "outro"
)

var professionalType = stringEnum("Categoria do profissional de saúde", map[string]string{
	"Médico":       ProfessionalMedico,
	"Enfermeiro":   ProfessionalEnfermeiro,
	"Farmacêutico": ProfessionalFarmaceutico,
	"Outro":        ProfessionalOutro,
})

var recordType = stringEnum("Natureza do documento clínico", map[string]string{
	"Consulta":   "consulta",
	"Exame":      "exame",
	"Internação": "internacao",
	"Prescrição": "prescricao",
	"Laudo":      "laudo",
	"Outro":      "outro",
})

// Níveis de acesso concedidos pelo paciente.
const (
	AccessRead      = "leitura"
	AccessReadWrite = "leituraEscrita"
)

var accessLevel = stringEnum("Nível de acesso", map[string]string{
	"Leitura":           AccessRead,
	"Leitura e escrita": AccessReadWrite,
})

// Situações de uma permissão. "expirada" não é gravada: é calculada na leitura
// a partir de expiresAt.
const (
	PermissionActive  = "ativa"
	PermissionRevoked = "revogada"
	PermissionExpired = "expirada"
)

var permissionStatus = stringEnum("Situação da permissão", map[string]string{
	"Ativa":    PermissionActive,
	"Revogada": PermissionRevoked,
	"Expirada": PermissionExpired,
})

// Situações de um acesso extraordinário.
const (
	EmergencyActive  = "ativa"
	EmergencyUsed    = "utilizada"
	EmergencyExpired = "expirada"
)

var emergencyStatus = stringEnum("Situação do acesso extraordinário", map[string]string{
	"Ativa":     EmergencyActive,
	"Utilizada": EmergencyUsed,
	"Expirada":  EmergencyExpired,
})

// Operações registradas na trilha de auditoria.
const (
	OpCreate           = "CRIACAO"
	OpRead             = "LEITURA"
	OpEmergencyRead    = "LEITURA_EMERGENCIAL"
	OpEmergencyRequest = "SOLICITACAO_EMERGENCIAL"
	OpGrant            = "CONCESSAO"
	OpRevoke           = "REVOGACAO"
	OpInvalidate       = "INVALIDACAO"
	OpDenied           = "ACESSO_NEGADO"
)

var operationType = stringEnum("Operação auditada", map[string]string{
	"Criação":                 OpCreate,
	"Leitura":                 OpRead,
	"Leitura emergencial":     OpEmergencyRead,
	"Solicitação emergencial": OpEmergencyRequest,
	"Concessão":               OpGrant,
	"Revogação":               OpRevoke,
	"Invalidação":             OpInvalidate,
	"Acesso negado":           OpDenied,
})
