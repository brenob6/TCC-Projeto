package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
)

// GetPatientByCpf localiza o paciente pelo CPF, recalculando o HMAC.
var GetPatientByCpf = tx.Transaction{
	Tag:         "getPatientByCpf",
	Label:       "Consultar paciente por CPF",
	Description: "Localiza o paciente a partir do CPF, sem expô-lo no ledger",
	Method:      "GET",
	ReadOnly:    true,
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
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		cpf, _ := req["cpf"].(string)
		cpfHash, err := hashCPF(cpf)
		if err != nil {
			return nil, err
		}

		patient, err := getAssetByKeyProps(stub, "patient", map[string]interface{}{
			"id": deterministicID("patient", cpfHash),
		}, "paciente")
		if err != nil {
			return nil, err
		}

		patientJSON, nerr := json.Marshal(patient)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return patientJSON, nil
	},
}
