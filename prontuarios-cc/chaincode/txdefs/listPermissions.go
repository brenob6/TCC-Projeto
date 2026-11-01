package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"

	"prontuarios-cc/datatypes"
)

// ListPermissions lista as permissões do paciente com a situação efetiva
// (permissões vencidas aparecem como "expirada").
var ListPermissions = tx.Transaction{
	Tag:         "listPermissions",
	Label:       "Listar permissões",
	Description: "Lista os consentimentos concedidos pelo paciente",
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
			Tag:      "onlyActive",
			Label:    "Somente ativas",
			DataType: "boolean",
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		patientKey, _ := req["patient"].(assets.Key)
		onlyActive, _ := req["onlyActive"].(bool)

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

		perms, err := referrers(stub, patientKey, "accessPermission")
		if err != nil {
			return nil, err
		}
		sortByTime(perms, "grantedAt")

		result := []*assets.Asset{}
		for _, perm := range perms {
			status := permissionStatusAt(perm, now)
			if onlyActive && status != datatypes.PermissionActive {
				continue
			}
			(*perm)["status"] = status
			result = append(result, perm)
		}

		resJSON, nerr := json.Marshal(result)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
