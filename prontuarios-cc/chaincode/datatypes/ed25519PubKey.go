package datatypes

import (
	"crypto/ed25519"
	"encoding/base64"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
)

// ed25519PubKey é a chave pública com que o paciente assina consentimentos,
// codificada em Base64 padrão (32 bytes).
var ed25519PubKey = assets.DataType{
	AcceptedFormats: []string{"string"},
	Description:     "Chave pública Ed25519 em Base64 (32 bytes)",
	Parse: func(data interface{}) (string, interface{}, errors.ICCError) {
		value, ok := data.(string)
		if !ok {
			return "", nil, errors.NewCCError("a chave pública deve ser uma string", 400)
		}
		raw, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return "", nil, errors.NewCCError("a chave pública deve ser Ed25519 em Base64 (32 bytes)", 400)
		}
		return value, value, nil
	},
}
