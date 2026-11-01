package txdefs

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"

	"prontuarios-cc/datatypes"
)

// DefaultAuthorizationMaxAge é a validade padrão de uma autorização de
// leitura para o KMS.
const DefaultAuthorizationMaxAge = 5 * time.Minute

// GetAccessAuthorization permite que o KMS e os custodiantes confirmem, antes
// da decifração, que o accessLog informado pela API registra uma leitura
// autorizada, recente e relativa à chave solicitada.
var GetAccessAuthorization = tx.Transaction{
	Tag:         "getAccessAuthorization",
	Label:       "Validar autorização de leitura",
	Description: "Confirma que um registro de auditoria autoriza a decifração",
	Method:      "GET",
	ReadOnly:    true,
	Callers:     institutionCallers,

	Args: []tx.Argument{
		{
			Tag:      "accessLog",
			Label:    "Registro de auditoria",
			DataType: "->accessLog",
			Required: true,
		},
		{
			Tag:         "maxAgeSeconds",
			Label:       "Validade (s)",
			Description: "Idade máxima do registro; padrão 300",
			DataType:    "integer",
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		logKey, _ := req["accessLog"].(assets.Key)
		maxAge := DefaultAuthorizationMaxAge
		if v, ok := req["maxAgeSeconds"]; ok {
			maxAge = time.Duration(toInt(v)) * time.Second
		}

		logAsset, err := getAsset(stub, logKey, "registro de auditoria")
		if err != nil {
			return nil, err
		}
		now, err := txTime(stub)
		if err != nil {
			return nil, err
		}

		operation, _ := (*logAsset)["operation"].(string)
		occurredAt, _ := parseTime((*logAsset)["occurredAt"])
		res := map[string]interface{}{
			"operation":   operation,
			"patient":     refKey((*logAsset)["patient"]),
			"actor":       refKey((*logAsset)["actor"]),
			"institution": refKey((*logAsset)["institution"]),
			"occurredAt":  (*logAsset)["occurredAt"],
		}

		valid := operation == datatypes.OpRead || operation == datatypes.OpEmergencyRead
		if valid && now.Sub(occurredAt) > maxAge {
			valid = false
			res["reason"] = "autorização expirada"
		} else if !valid {
			res["reason"] = "o registro não corresponde a uma leitura autorizada"
		}
		if valid {
			record, err := getAssetByKey(stub, "medicalRecord", refKey((*logAsset)["record"]), "prontuário")
			if err != nil {
				return nil, err
			}
			res["cid"] = (*record)["cid"]
			res["contentHash"] = (*record)["contentHash"]
			res["kmsKeyId"] = (*record)["kmsKeyId"]
			res["emergency"] = operation == datatypes.OpEmergencyRead
		}
		res["valid"] = valid

		resJSON, nerr := json.Marshal(res)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
