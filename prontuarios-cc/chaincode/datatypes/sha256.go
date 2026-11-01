package datatypes

import (
	"encoding/hex"
	"strings"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
)

var sha256 = assets.DataType{
	AcceptedFormats: []string{"string"},
	Description:     "Resumo SHA-256 em hexadecimal (64 caracteres)",
	Parse: func(data interface{}) (string, interface{}, errors.ICCError) {
		value, ok := data.(string)
		if !ok {
			return "", nil, errors.NewCCError("o hash deve ser uma string", 400)
		}
		value = strings.ToLower(value)
		if raw, err := hex.DecodeString(value); err != nil || len(raw) != 32 {
			return "", nil, errors.NewCCError("o hash deve ser um SHA-256 em hexadecimal (64 caracteres)", 400)
		}
		return value, value, nil
	},
}
