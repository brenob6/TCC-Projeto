package server

import "fmt"

// Step registra uma interação entre a API e uma camada da arquitetura. A
// interface exibe a sequência para acompanhar os fluxos das Figuras 7 a 9.
type Step struct {
	Layer  string `json:"layer"` // API, Blockchain, IPFS ou KMS
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
	TxID   string `json:"txId,omitempty"`
}

// Trace acumula os passos de uma operação.
type Trace struct {
	Steps []Step `json:"steps"`
}

func (t *Trace) add(layer, action, format string, a ...any) {
	t.Steps = append(t.Steps, Step{Layer: layer, Action: action, Detail: fmt.Sprintf(format, a...)})
}

func (t *Trace) tx(action, txID, format string, a ...any) {
	t.Steps = append(t.Steps, Step{Layer: "Blockchain", Action: action, TxID: txID, Detail: fmt.Sprintf(format, a...)})
}
