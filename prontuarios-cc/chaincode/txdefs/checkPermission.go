package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
)

// CheckPermission responde, sem registrar auditoria, se o profissional pode ler
// ou escrever prontuários do paciente. Serve à interface; a decifração sempre
// passa por requestRecordAccess.
var CheckPermission = tx.Transaction{
	Tag:         "checkPermission",
	Label:       "Verificar permissão",
	Description: "Consulta somente leitura das permissões do profissional",
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
			Tag:      "professional",
			Label:    "Profissional",
			DataType: "->professional",
			Required: true,
		},
		{
			Tag:      "record",
			Label:    "Prontuário",
			DataType: "->medicalRecord",
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		patientKey, _ := req["patient"].(assets.Key)
		profKey, _ := req["professional"].(assets.Key)

		prof, _, _, err := callerProfessional(stub, profKey)
		if err != nil {
			return nil, err
		}
		patient, err := activePatient(stub, patientKey)
		if err != nil {
			return nil, err
		}
		now, err := txTime(stub)
		if err != nil {
			return nil, err
		}

		var record *assets.Asset
		if recordKey, ok := req["record"].(assets.Key); ok {
			if record, err = recordOfPatient(stub, recordKey, patient); err != nil {
				return nil, err
			}
		}

		res := map[string]interface{}{"read": false, "write": false}
		if record == nil || !isInvalidated(record) {
			readPerm, err := findPermission(stub, patient, prof, record, false, now)
			if err != nil {
				return nil, err
			}
			if readPerm != nil {
				res["read"] = true
				res["permission"] = readPerm.Key()
			} else if record != nil {
				em, err := findEmergency(stub, record, prof, now)
				if err != nil {
					return nil, err
				}
				if em != nil {
					res["read"] = true
					res["emergency"] = em.Key()
				}
			}
		}
		writePerm, err := findPermission(stub, patient, prof, nil, true, now)
		if err != nil {
			return nil, err
		}
		res["write"] = writePerm != nil

		resJSON, nerr := json.Marshal(res)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
