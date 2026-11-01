package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
)

// ListEmergencyAccesses lista os acessos extraordinários do paciente para
// revisão posterior pelo titular e pelas instituições.
var ListEmergencyAccesses = tx.Transaction{
	Tag:         "listEmergencyAccesses",
	Label:       "Listar acessos extraordinários",
	Description: "Lista os acessos emergenciais aos prontuários do paciente",
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
		if _, err := getAsset(stub, patientKey, "paciente"); err != nil {
			return nil, err
		}
		now, err := txTime(stub)
		if err != nil {
			return nil, err
		}

		ems, err := referrers(stub, patientKey, "emergencyAccess")
		if err != nil {
			return nil, err
		}
		sortByTime(ems, "requestedAt")

		result := []*assets.Asset{}
		for _, em := range ems {
			if !inPeriod(em, "requestedAt", req) {
				continue
			}
			(*em)["status"] = emergencyStatusAt(em, now)
			result = append(result, em)
		}

		resJSON, nerr := json.Marshal(result)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
