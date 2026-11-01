package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
)

// RegisterInstitution cadastra a instituição representada pela organização
// chamadora (RF-01). Cada MSP corresponde a uma única instituição.
var RegisterInstitution = tx.Transaction{
	Tag:         "registerInstitution",
	Label:       "Cadastrar instituição",
	Description: "Cadastra a instituição de saúde da organização chamadora",
	Method:      "POST",
	Callers:     institutionCallers,

	Args: []tx.Argument{
		{
			Tag:         "id",
			Label:       "CNES",
			Description: "Código CNES da instituição",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:      "name",
			Label:    "Nome",
			DataType: "string",
			Required: true,
		},
		{
			Tag:      "cnpj",
			Label:    "CNPJ",
			DataType: "cnpj",
			Required: true,
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		id, _ := req["id"].(string)
		name, _ := req["name"].(string)
		cnpj, _ := req["cnpj"].(string)
		if id == "" || name == "" {
			return nil, errors.NewCCError("CNES e nome são obrigatórios", 400)
		}

		msp, err := stub.GetMSPID()
		if err != nil {
			return nil, errors.WrapErrorWithStatus(err, "falha ao identificar o chamador", 500)
		}
		indexKey, err := stub.CreateCompositeKey(mspIndex, []string{msp})
		if err != nil {
			return nil, errors.WrapError(err, "falha ao montar o índice de MSP")
		}
		existing, err := stub.GetState(indexKey)
		if err != nil {
			return nil, errors.WrapError(err, "falha ao consultar o índice de MSP")
		}
		if len(existing) > 0 {
			return nil, errors.NewCCError("a organização chamadora já possui instituição cadastrada", 409)
		}

		now, err := txTime(stub)
		if err != nil {
			return nil, err
		}
		inst, err := assets.NewAsset(map[string]interface{}{
			"@assetType":   "institution",
			"id":           id,
			"name":         name,
			"cnpj":         cnpj,
			"mspId":        msp,
			"active":       true,
			"registeredAt": formatTime(now),
		})
		if err != nil {
			return nil, errors.WrapError(err, "falha ao montar a instituição")
		}
		if _, err = inst.PutNew(stub); err != nil {
			return nil, errors.WrapError(err, "falha ao gravar a instituição")
		}
		if err = stub.PutState(indexKey, []byte(inst.Key())); err != nil {
			return nil, errors.WrapError(err, "falha ao gravar o índice de MSP")
		}

		instJSON, nerr := json.Marshal(inst)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return instJSON, nil
	},
}
