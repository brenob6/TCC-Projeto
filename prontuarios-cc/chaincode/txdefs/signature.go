package txdefs

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
)

// signatureDomain separa as mensagens deste chaincode de qualquer outro uso da
// chave do paciente.
const signatureDomain = "prontuarios-cc/v1"

// signatureArgs são os argumentos comuns às transações assinadas pelo paciente.
var signatureArgs = []tx.Argument{
	{
		Tag:         "nonce",
		Label:       "Nonce",
		Description: "signatureNonce atual do paciente + 1",
		DataType:    "integer",
		Required:    true,
	},
	{
		Tag:         "signature",
		Label:       "Assinatura",
		Description: "Assinatura Ed25519 (Base64) da mensagem canônica",
		DataType:    "string",
		Required:    true,
	},
}

// SignedMessage monta a mensagem que o paciente assina: um objeto JSON com
// chaves em ordem alfabética, sem espaços e sem escape de HTML. payload contém
// os campos específicos da operação.
func SignedMessage(action, patientID string, nonce int64, payload map[string]interface{}) ([]byte, error) {
	msg := map[string]interface{}{
		"domain":  signatureDomain,
		"action":  action,
		"patient": patientID,
		"nonce":   nonce,
	}
	for k, v := range payload {
		msg[k] = v
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(msg); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// verifyPatientSignature confere a assinatura e consome o nonce, impedindo que
// a mesma mensagem seja reaproveitada.
func verifyPatientSignature(stub *sw.StubWrapper, patient *assets.Asset, action string, req map[string]interface{}, payload map[string]interface{}) (string, errors.ICCError) {
	nonce := toInt(req["nonce"])
	signature, _ := req["signature"].(string)

	expected := toInt((*patient)["signatureNonce"]) + 1
	if nonce != expected {
		return "", errors.NewCCError(fmt.Sprintf("nonce inválido: esperado %d", expected), 409)
	}

	pubB64, _ := (*patient)["signingPublicKey"].(string)
	pub, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return "", errors.NewCCError("chave de assinatura do paciente inválida", 500)
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return "", errors.NewCCError("assinatura deve estar em Base64", 400)
	}

	patientID, _ := (*patient)["id"].(string)
	msg, err := SignedMessage(action, patientID, nonce, payload)
	if err != nil {
		return "", errors.WrapError(err, "falha ao montar a mensagem assinada")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), msg, sig) {
		return "", errors.NewCCError("assinatura do paciente inválida", 401)
	}

	patientKey := assets.Key{"@assetType": "patient", "@key": patient.Key()}
	if _, err := patientKey.Update(stub, map[string]interface{}{"signatureNonce": float64(nonce)}); err != nil {
		return "", errors.WrapError(err, "falha ao atualizar o nonce do paciente")
	}
	return signature, nil
}
