package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
)

// GetAccessHistory devolve a trilha de auditoria do paciente ou de um
// prontuário, em ordem cronológica (RF-08).
var GetAccessHistory = tx.Transaction{
	Tag:         "getAccessHistory",
	Label:       "Histórico de acessos",
	Description: "Lista quem acessou, de qual instituição, qual operação e quando",
	Method:      "GET",
	ReadOnly:    true,
	Callers:     institutionCallers,

	Args: []tx.Argument{
		{
			Tag:      "patient",
			Label:    "Paciente",
			DataType: "->patient",
			Required: true,
		},
		{
			Tag:         "record",
			Label:       "Prontuário",
			Description: "Restringe o histórico a um prontuário",
			DataType:    "->medicalRecord",
		},
		{
			Tag:      "from",
			Label:    "De",
			DataType: "datetime",
		},
		{
			Tag:      "to",
			Label:    "Até",
			DataType: "datetime",
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		patientKey, _ := req["patient"].(assets.Key)

		if _, _, err := callerInstitution(stub); err != nil {
			return nil, err
		}
		patient, err := getAsset(stub, patientKey, "paciente")
		if err != nil {
			return nil, err
		}

		source := patientKey
		if recordKey, ok := req["record"].(assets.Key); ok {
			if _, err := recordOfPatient(stub, recordKey, patient); err != nil {
				return nil, err
			}
			source = recordKey
		}

		logs, err := referrers(stub, source, "accessLog")
		if err != nil {
			return nil, err
		}
		sortByTime(logs, "occurredAt")

		result := []*assets.Asset{}
		for _, l := range logs {
			if inPeriod(l, "occurredAt", req) {
				result = append(result, l)
			}
		}

		resJSON, nerr := json.Marshal(result)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
