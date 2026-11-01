package txdefs

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"

	"prontuarios-cc/datatypes"
)

// RequestEmergencyAccess registra a justificativa e cria uma permissão
// extraordinária de uso único, válida por EmergencyWindow (exRecover da
// Figura 9, RF-06). A leitura em si ocorre em requestRecordAccess, que consome
// a permissão.
var RequestEmergencyAccess = tx.Transaction{
	Tag:         "requestEmergencyAccess",
	Label:       "Solicitar acesso extraordinário",
	Description: "Registra o motivo e libera uma leitura emergencial do prontuário",
	Method:      "POST",
	Callers:     institutionCallers,

	Args: []tx.Argument{
		{
			Tag:      "patient",
			Label:    "Paciente",
			DataType: "->patient",
			Required: true,
		},
		{
			Tag:      "professional",
			Label:    "Profissional",
			DataType: "->professional",
			Required: true,
		},
		{
			Tag:      "record",
			Label:    "Prontuário",
			DataType: "->medicalRecord",
			Required: true,
		},
		{
			Tag:         "reason",
			Label:       "Justificativa",
			Description: "Ex.: paciente inconsciente",
			DataType:    "string",
			Required:    true,
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		patientKey, _ := req["patient"].(assets.Key)
		profKey, _ := req["professional"].(assets.Key)
		recordKey, _ := req["record"].(assets.Key)
		reason, _ := req["reason"].(string)
		reason = strings.TrimSpace(reason)

		if len([]rune(reason)) < MinEmergencyReasonLength {
			return nil, errors.NewCCError(fmt.Sprintf("a justificativa deve ter ao menos %d caracteres", MinEmergencyReasonLength), 400)
		}

		prof, inst, msp, err := callerProfessional(stub, profKey)
		if err != nil {
			return nil, err
		}
		if allowed, _ := (*prof)["emergencyAccessAllowed"].(bool); !allowed {
			return nil, errors.NewCCError("o profissional não está habilitado para acesso extraordinário", 403)
		}
		patient, err := activePatient(stub, patientKey)
		if err != nil {
			return nil, err
		}
		record, err := recordOfPatient(stub, recordKey, patient)
		if err != nil {
			return nil, err
		}
		if isInvalidated(record) {
			return nil, errors.NewCCError("o prontuário foi eliminado a pedido do titular", 410)
		}
		now, err := txTime(stub)
		if err != nil {
			return nil, err
		}

		em, err := assets.NewAsset(map[string]interface{}{
			"@assetType":   "emergencyAccess",
			"id":           stub.Stub.GetTxID(),
			"patient":      ref(patient),
			"professional": ref(prof),
			"institution":  ref(inst),
			"record":       ref(record),
			"reason":       reason,
			"requestedAt":  formatTime(now),
			"expiresAt":    formatTime(now.Add(EmergencyWindow)),
			"status":       datatypes.EmergencyActive,
		})
		if err != nil {
			return nil, errors.WrapError(err, "falha ao montar o acesso extraordinário")
		}
		if _, err = em.PutNew(stub); err != nil {
			return nil, errors.WrapError(err, "falha ao gravar o acesso extraordinário")
		}

		logAsset, err := writeAudit(stub, inst, msp, auditEntry{
			Operation: datatypes.OpEmergencyRequest,
			Patient:   patient,
			Record:    record,
			Actor:     prof,
			Emergency: &em,
			Details:   reason,
		})
		if err != nil {
			return nil, err
		}

		resJSON, nerr := json.Marshal(map[string]interface{}{
			"emergencyAccess": em,
			"accessLog":       logAsset.Key(),
		})
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
