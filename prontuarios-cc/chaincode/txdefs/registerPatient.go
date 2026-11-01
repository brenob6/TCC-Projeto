package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
)

// RegisterPatient cadastra um paciente (RF-02). O CPF chega pelo mapa
// transiente e não é gravado no bloco; o ledger guarda apenas o HMAC.
// Pressupõe que a API já gerou a chave do paciente no KMS (kms keygen).
var RegisterPatient = tx.Transaction{
	Tag:         "registerPatient",
	Label:       "Cadastrar paciente",
	Description: "Cadastra um paciente com identificador pseudônimo",
	Method:      "POST",
	Callers:     institutionCallers,

	Args: []tx.Argument{
		{
			Tag:         "cpf",
			Label:       "CPF",
			Description: "Enviado no mapa transiente (@request)",
			DataType:    "cpf",
			Required:    true,
			Private:     true,
		},
		{
			Tag:         "kmsKeyId",
			Label:       "Chave no KMS",
			Description: "Identificador usado em kms keygen",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:         "publicKey",
			Label:       "Chave pública de cifração",
			Description: "Chave pública ElGamal devolvida pelo KMS",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:         "signingPublicKey",
			Label:       "Chave pública de assinatura",
			Description: "Chave Ed25519 do paciente, em Base64",
			DataType:    "ed25519PubKey",
			Required:    true,
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		cpf, _ := req["cpf"].(string)
		kmsKeyID, _ := req["kmsKeyId"].(string)
		publicKey, _ := req["publicKey"].(string)
		signingKey, _ := req["signingPublicKey"].(string)
		if kmsKeyID == "" || publicKey == "" {
			return nil, errors.NewCCError("kmsKeyId e publicKey são obrigatórios", 400)
		}

		if _, _, err := callerInstitution(stub); err != nil {
			return nil, err
		}
		cpfHash, err := hashCPF(cpf)
		if err != nil {
			return nil, err
		}
		now, err := txTime(stub)
		if err != nil {
			return nil, err
		}

		patient, err := assets.NewAsset(map[string]interface{}{
			"@assetType":       "patient",
			"id":               deterministicID("patient", cpfHash),
			"cpfHash":          cpfHash,
			"kmsKeyId":         kmsKeyID,
			"publicKey":        publicKey,
			"signingPublicKey": signingKey,
			"signatureNonce":   0,
			"active":           true,
			"registeredAt":     formatTime(now),
		})
		if err != nil {
			return nil, errors.WrapError(err, "falha ao montar o paciente")
		}
		exists, err := patient.ExistsInLedger(stub)
		if err != nil {
			return nil, errors.WrapError(err, "falha ao verificar o paciente")
		}
		if exists {
			return nil, errors.NewCCError("paciente já cadastrado", 409)
		}
		if _, err = patient.Put(stub); err != nil {
			return nil, errors.WrapError(err, "falha ao gravar o paciente")
		}

		patientJSON, nerr := json.Marshal(patient)
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return patientJSON, nil
	},
}
