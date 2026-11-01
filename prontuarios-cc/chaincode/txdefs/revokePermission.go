package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"

	"prontuarios-cc/datatypes"
)

// RevokePermission revoga um consentimento (RF-04). O efeito é imediato:
// requestRecordAccess deixa de considerar a permissão.
var RevokePermission = tx.Transaction{
	Tag:         "revokePermission",
	Label:       "Revogar permissão",
	Description: "Revoga uma permissão, com assinatura do paciente",
	Method:      "PUT",
	Callers:     institutionCallers,

	Args: append([]tx.Argument{
		{
			Tag:      "permission",
			Label:    "Permissão",
			DataType: "->accessPermission",
			Required: true,
		},
	}, signatureArgs...),
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		permKey, _ := req["permission"].(assets.Key)

		inst, msp, err := callerInstitution(stub)
		if err != nil {
			return nil, err
		}
		perm, err := getAsset(stub, permKey, "permissão")
		if err != nil {
			return nil, err
		}
		if (*perm)["status"] != datatypes.PermissionActive {
			return nil, errors.NewCCError("a permissão já foi revogada", 409)
		}
		patient, err := getAssetByKey(stub, "patient", refKey((*perm)["patient"]), "paciente")
		if err != nil {
			return nil, err
		}

		if _, err := verifyPatientSignature(stub, patient, "revokePermission", req, map[string]interface{}{
			"permission": (*perm)["id"],
		}); err != nil {
			return nil, err
		}

		now, err := txTime(stub)
		if err != nil {
			return nil, err
		}
		updated, err := permKey.Update(stub, map[string]interface{}{
			"status":    datatypes.PermissionRevoked,
			"revokedAt": formatTime(now),
		})
		if err != nil {
			return nil, errors.WrapError(err, "falha ao revogar a permissão")
		}
		updatedPerm := assets.Asset(updated)

		logAsset, err := writeAudit(stub, inst, msp, auditEntry{
			Operation:  datatypes.OpRevoke,
			Patient:    patient,
			Permission: &updatedPerm,
		})
		if err != nil {
			return nil, err
		}

		resJSON, nerr := json.Marshal(map[string]interface{}{
			"permission": updated,
			"accessLog":  logAsset.Key(),
		})
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
