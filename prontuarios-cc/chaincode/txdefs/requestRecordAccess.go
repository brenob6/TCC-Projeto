package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"

	"prontuarios-cc/datatypes"
)

// RequestRecordAccess decide e audita a leitura de um prontuário
// (hasPermission das Figuras 8 e 9). É uma transação de escrita para que toda
// leitura, autorizada ou negada, fique registrada no ledger (RF-03).
//
// A autorização vem de uma permissão ativa do paciente ou, na falta dela, de
// um acesso extraordinário ativo, que é consumido nesta transação
// (removeExPermission). Uma negativa não aborta a transação: é gravada como
// ACESSO_NEGADO e devolvida com granted=false.
var RequestRecordAccess = tx.Transaction{
	Tag:         "requestRecordAccess",
	Label:       "Solicitar leitura de prontuário",
	Description: "Verifica a permissão, registra a leitura e devolve os dados para decifração",
	Method:      "POST",
	Callers:     institutionCallers,

	Args: []tx.Argument{
		{
			Tag:      "record",
			Label:    "Prontuário",
			DataType: "->medicalRecord",
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
		recordKey, _ := req["record"].(assets.Key)
		profKey, _ := req["professional"].(assets.Key)

		prof, inst, msp, err := callerProfessional(stub, profKey)
		if err != nil {
			return nil, err
		}
		record, err := getAsset(stub, recordKey, "prontuário")
		if err != nil {
			return nil, err
		}
		patient, err := getAssetByKey(stub, "patient", refKey((*record)["patient"]), "paciente")
		if err != nil {
			return nil, err
		}
		now, err := txTime(stub)
		if err != nil {
			return nil, err
		}

		entry := auditEntry{Patient: patient, Record: record, Actor: prof}
		switch {
		case isInvalidated(record):
			entry.Details = "prontuário eliminado a pedido do titular"
		case (*patient)["active"] != true:
			entry.Details = "paciente inativo"
		default:
			entry.Permission, err = findPermission(stub, patient, prof, record, false, now)
			if err != nil {
				return nil, err
			}
			if entry.Permission == nil {
				entry.Emergency, err = findEmergency(stub, record, prof, now)
				if err != nil {
					return nil, err
				}
			}
		}

		switch {
		case entry.Permission != nil:
			entry.Operation = datatypes.OpRead
		case entry.Emergency != nil:
			entry.Operation = datatypes.OpEmergencyRead
			emKey := assets.Key{"@assetType": "emergencyAccess", "@key": entry.Emergency.Key()}
			if _, err := emKey.Update(stub, map[string]interface{}{
				"status": datatypes.EmergencyUsed,
				"usedAt": formatTime(now),
			}); err != nil {
				return nil, errors.WrapError(err, "falha ao consumir o acesso extraordinário")
			}
		default:
			entry.Operation = datatypes.OpDenied
			if entry.Details == "" {
				entry.Details = "sem permissão do paciente nem acesso extraordinário ativo"
			}
		}

		logAsset, err := writeAudit(stub, inst, msp, entry)
		if err != nil {
			return nil, err
		}

		res := map[string]interface{}{
			"granted":   entry.Operation != datatypes.OpDenied,
			"operation": entry.Operation,
			"accessLog": logAsset.Key(),
		}
		if entry.Operation == datatypes.OpDenied {
			res["reason"] = entry.Details
		} else {
			res["cid"] = (*record)["cid"]
			res["contentHash"] = (*record)["contentHash"]
			res["kmsKeyId"] = (*record)["kmsKeyId"]
		}

		resJSON, nerr := json.Marshal(res)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
