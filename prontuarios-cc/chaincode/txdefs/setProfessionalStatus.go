package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
)

// SetProfessionalStatus encerra ou restabelece o vínculo do profissional e
// ajusta a habilitação para o acesso extraordinário.
var SetProfessionalStatus = tx.Transaction{
	Tag:         "setProfessionalStatus",
	Label:       "Alterar situação do profissional",
	Description: "Ativa ou desativa o profissional; apenas a instituição de vínculo",
	Method:      "PUT",
	Callers:     institutionCallers,

	Args: []tx.Argument{
		{
			Tag:      "professional",
			Label:    "Profissional",
			DataType: "->professional",
			Required: true,
		},
		{
			Tag:      "active",
			Label:    "Ativo",
			DataType: "boolean",
			Required: true,
		},
		{
			Tag:      "emergencyAccessAllowed",
			Label:    "Acesso extraordinário",
			DataType: "boolean",
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		profKey, _ := req["professional"].(assets.Key)

		inst, _, err := callerInstitution(stub)
		if err != nil {
			return nil, err
		}
		prof, err := getAsset(stub, profKey, "profissional")
		if err != nil {
			return nil, err
		}
		if refKey((*prof)["institution"]) != inst.Key() {
			return nil, errors.NewCCError("somente a instituição de vínculo altera o profissional", 403)
		}

		update := map[string]interface{}{"active": req["active"]}
		if v, ok := req["emergencyAccessAllowed"].(bool); ok {
			update["emergencyAccessAllowed"] = v
		}
		updated, err := profKey.Update(stub, update)
		if err != nil {
			return nil, errors.WrapError(err, "falha ao atualizar o profissional")
		}

		profJSON, nerr := json.Marshal(updated)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return profJSON, nil
	},
}
