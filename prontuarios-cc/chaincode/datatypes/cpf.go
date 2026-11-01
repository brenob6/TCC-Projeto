package datatypes

import (
	"strings"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
)

// cpf aceita o número com ou sem pontuação e o devolve somente com dígitos.
// É usado apenas como argumento de transação: o ledger guarda o hash.
var cpf = assets.DataType{
	AcceptedFormats: []string{"string"},
	Description:     "CPF (11 dígitos, com dígitos verificadores)",
	Parse: func(data interface{}) (string, interface{}, errors.ICCError) {
		value, ok := data.(string)
		if !ok {
			return "", nil, errors.NewCCError("o CPF deve ser uma string", 400)
		}
		value = strings.NewReplacer(".", "", "-", "").Replace(value)

		if !onlyDigits(value, 11) {
			return "", nil, errors.NewCCError("o CPF deve ter 11 dígitos", 400)
		}
		if allEqual(value) {
			return "", nil, errors.NewCCError("CPF inválido", 400)
		}
		if checkDigit(value[:9], 10) != value[9] || checkDigit(value[:10], 11) != value[10] {
			return "", nil, errors.NewCCError("CPF inválido", 400)
		}

		return value, value, nil
	},
}

// checkDigit calcula o dígito verificador do CPF com pesos decrescentes a
// partir de firstWeight.
func checkDigit(digits string, firstWeight int) byte {
	sum := 0
	for i, d := range digits {
		sum += (firstWeight - i) * int(d-'0')
	}
	rest := 11 - sum%11
	if rest > 9 {
		rest = 0
	}
	return byte('0' + rest)
}

func onlyDigits(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func allEqual(s string) bool {
	return strings.Count(s, s[:1]) == len(s)
}
