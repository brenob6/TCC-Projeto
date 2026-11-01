package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"

	"prontuarios-cc/datatypes"
)

// RegisterProfessional vincula um profissional de saúde à instituição da
// organização chamadora (RF-02).
var RegisterProfessional = tx.Transaction{
	Tag:         "registerProfessional",
	Label:       "Cadastrar profissional",
	Description: "Cadastra um profissional vinculado à instituição chamadora",
	Method:      "POST",
	Callers:     institutionCallers,

	Args: []tx.Argument{
		{
			Tag:         "cpf",
			Label:       "CPF",
			Description: "Enviado no mapa transiente (@request)",
			DataType:    "cpf",
			Required:    true,
			Private:     true,
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
			Description: "Padrão: verdadeiro para médicos",
			DataType:    "boolean",
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		cpf, _ := req["cpf"].(string)
		name, _ := req["name"].(string)
		profType, _ := req["professionalType"].(string)
		registry, _ := req["councilRegistry"].(string)
		instKey, _ := req["institution"].(assets.Key)
		emergency, hasEmergency := req["emergencyAccessAllowed"].(bool)
		if !hasEmergency {
			emergency = profType == datatypes.ProfessionalMedico
		}
		if name == "" || registry == "" {
			return nil, errors.NewCCError("nome e registro no conselho são obrigatórios", 400)
		}

		inst, _, err := callerInstitution(stub)
		if err != nil {
			return nil, err
		}
		if inst.Key() != instKey.Key() {
			return nil, errors.NewCCError("o profissional só pode ser vinculado à instituição do chamador", 403)
		}
		cpfHash, err := hashCPF(cpf)
		if err != nil {
			return nil, err
		}
		now, err := txTime(stub)
		if err != nil {
			return nil, err
		}

		prof, err := assets.NewAsset(map[string]interface{}{
			"@assetType":             "professional",
			"id":                     deterministicID("professional", cpfHash, inst.Key()),
			"cpfHash":                cpfHash,
			"name":                   name,
			"professionalType":       profType,
			"councilRegistry":        registry,
			"institution":            ref(inst),
			"emergencyAccessAllowed": emergency,
			"active":                 true,
			"registeredAt":           formatTime(now),
		})
		if err != nil {
			return nil, errors.WrapError(err, "falha ao montar o profissional")
		}
		exists, err := prof.ExistsInLedger(stub)
		if err != nil {
			return nil, errors.WrapError(err, "falha ao verificar o profissional")
		}
		if exists {
			return nil, errors.NewCCError("profissional já cadastrado nesta instituição", 409)
		}
		if _, err = prof.Put(stub); err != nil {
			return nil, errors.WrapError(err, "falha ao gravar o profissional")
		}

		profJSON, nerr := json.Marshal(prof)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return profJSON, nil
	},
}
