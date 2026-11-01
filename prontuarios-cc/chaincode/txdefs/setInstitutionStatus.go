package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
)

// SetInstitutionStatus ativa ou desativa a instituição. Uma instituição
// inativa não submete transações de negócio.
var SetInstitutionStatus = tx.Transaction{
	Tag:         "setInstitutionStatus",
	Label:       "Alterar situação da instituição",
	Description: "Ativa ou desativa a instituição da organização chamadora",
	Method:      "PUT",
	Callers:     institutionCallers,

	Args: []tx.Argument{
		{
			Tag:      "institution",
			Label:    "Instituição",
			DataType: "->institution",
			Required: true,
		},
		{
			Tag:      "active",
			Label:    "Ativa",
			DataType: "boolean",
			Required: true,
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		instKey, _ := req["institution"].(assets.Key)
		active, _ := req["active"].(bool)

		inst, err := getAsset(stub, instKey, "instituição")
		if err != nil {
			return nil, err
		}
		msp, err := stub.GetMSPID()
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "falha ao identificar o chamador", 500)
		}
		if (*inst)["mspId"] != msp {
			return nil, errors.NewCCError("somente a própria organização altera a situação da instituição", 403)
		}

		updated, err := instKey.Update(stub, map[string]interface{}{"active": active})
		if err != nil {
			return nil, errors.WrapError(err, "falha ao atualizar a instituição")
		}

		instJSON, nerr := json.Marshal(updated)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return instJSON, nil
	},
}
