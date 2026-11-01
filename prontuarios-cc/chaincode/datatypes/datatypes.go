// Package datatypes define os tipos de dados próprios do chaincode, usados nas
// propriedades dos ativos e nos argumentos das transações.
package datatypes

import (
	"github.com/hyperledger-labs/cc-tools/assets"
)

// CustomDataTypes contém os tipos registrados no cc-tools.
var CustomDataTypes = map[string]assets.DataType{
	"cpf":              cpf,
	"cnpj":             cnpj,
	"sha256":           sha256,
	"ed25519PubKey":    ed25519PubKey,
	"professionalType": professionalType,
	"recordType":       recordType,
	"accessLevel":      accessLevel,
	"permissionStatus": permissionStatus,
	"emergencyStatus":  emergencyStatus,
	"operationType":    operationType,
}
