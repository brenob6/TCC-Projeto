package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"

	"prontuarios-cc/datatypes"
)

// InvalidateMedicalRecord realiza a eliminação lógica de um prontuário a
// pedido do titular (LGPD, art. 18). Exige a assinatura do paciente. A
// remoção do arquivo no IPFS (unpin) é responsabilidade da API.
var InvalidateMedicalRecord = tx.Transaction{
	Tag:         "invalidateMedicalRecord",
	Label:       "Eliminar prontuário",
	Description: "Eliminação lógica de um prontuário, assinada pelo paciente",
	Method:      "PUT",
	Callers:     institutionCallers,

	Args: append([]tx.Argument{
		{
			Tag:      "record",
			Label:    "Prontuário",
			DataType: "->medicalRecord",
			Required: true,
		},
		{
			Tag:      "reason",
			Label:    "Motivo",
			DataType: "string",
			Required: true,
		},
	}, signatureArgs...),
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		recordKey, _ := req["record"].(assets.Key)
		reason, _ := req["reason"].(string)

		inst, msp, err := callerInstitution(stub)
		if err != nil {
			return nil, err
		}
		record, err := getAsset(stub, recordKey, "prontuário")
		if err != nil {
			return nil, err
		}
		if isInvalidated(record) {
			return nil, errors.NewCCError("o prontuário já foi eliminado", 409)
		}
		patient, err := getAssetByKey(stub, "patient", refKey((*record)["patient"]), "paciente")
		if err != nil {
			return nil, err
		}

		if _, err := verifyPatientSignature(stub, patient, "invalidateMedicalRecord", req, map[string]interface{}{
			"record": (*record)["cid"],
			"reason": reason,
		}); err != nil {
			return nil, err
		}

		updated, err := recordKey.Update(stub, map[string]interface{}{"invalidated": true})
		if err != nil {
			return nil, errors.WrapError(err, "falha ao eliminar o prontuário")
		}
		updatedRecord := assets.Asset(updated)

		logAsset, err := writeAudit(stub, inst, msp, auditEntry{
			Operation: datatypes.OpInvalidate,
			Patient:   patient,
			Record:    &updatedRecord,
			Details:   reason,
		})
		if err != nil {
			return nil, err
		}

		resJSON, nerr := json.Marshal(map[string]interface{}{
			"record":    updated,
			"accessLog": logAsset.Key(),
		})
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
