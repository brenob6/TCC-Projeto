package datatypes

import (
	"strings"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
)

var cnpj = assets.DataType{
	AcceptedFormats: []string{"string"},
	Description:     "CNPJ (14 dígitos, com dígitos verificadores)",
	Parse: func(data interface{}) (string, interface{}, errors.ICCError) {
		value, ok := data.(string)
		if !ok {
			return "", nil, errors.NewCCError("o CNPJ deve ser uma string", 400)
		}
		value = strings.NewReplacer(".", "", "-", "", "/", "").Replace(value)

		if !onlyDigits(value, 14) {
			return "", nil, errors.NewCCError("o CNPJ deve ter 14 dígitos", 400)
		}
		if allEqual(value) {
			return "", nil, errors.NewCCError("CNPJ inválido", 400)
		}
		if cnpjDigit(value[:12]) != value[12] || cnpjDigit(value[:13]) != value[13] {
			return "", nil, errors.NewCCError("CNPJ inválido", 400)
		}

		return value, value, nil
	},
}

// cnpjDigit aplica os pesos 2..9 da direita para a esquerda.
func cnpjDigit(digits string) byte {
	sum, weight := 0, 2
	for i := len(digits) - 1; i >= 0; i-- {
		sum += int(digits[i]-'0') * weight
		weight++
		if weight > 9 {
			weight = 2
		}
	}
	rest := sum % 11
	if rest < 2 {
		return '0'
	}
	return byte('0' + 11 - rest)
}
