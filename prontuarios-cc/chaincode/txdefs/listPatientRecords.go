package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
)

// ListPatientRecords lista os metadados dos prontuários que o profissional
// pode ler. Não entrega conteúdo e, por ser consulta, não gera auditoria; a
// leitura em si passa por requestRecordAccess.
var ListPatientRecords = tx.Transaction{
	Tag:         "listPatientRecords",
	Label:       "Listar prontuários do paciente",
	Description: "Lista os prontuários acessíveis ao profissional",
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

		records, err := referrers(stub, patientKey, "medicalRecord")
		if err != nil {
			return nil, err
		}
		sortByTime(records, "createdAt")

		result := []map[string]interface{}{}
		for _, record := range records {
			if isInvalidated(record) {
				continue
			}
			perm, err := findPermission(stub, patient, prof, record, false, now)
			if err != nil {
				return nil, err
			}
			viaEmergency := false
			if perm == nil {
				em, err := findEmergency(stub, record, prof, now)
				if err != nil {
					return nil, err
				}
				if em == nil {
					continue
				}
				viaEmergency = true
			}
			result = append(result, map[string]interface{}{
				"cid":          (*record)["cid"],
				"recordType":   (*record)["recordType"],
				"author":       refKey((*record)["author"]),
				"institution":  refKey((*record)["institution"]),
				"createdAt":    (*record)["createdAt"],
				"viaEmergency": viaEmergency,
			})
		}

		resJSON, nerr := json.Marshal(result)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
