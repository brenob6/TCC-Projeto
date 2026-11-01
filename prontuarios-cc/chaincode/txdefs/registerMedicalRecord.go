package txdefs

import (
	"encoding/json"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"

	"prontuarios-cc/datatypes"
)

// RegisterMedicalRecord registra o CID de um prontuário já cifrado e
// armazenado no IPFS (registerCID da Figura 7). Exige consentimento de
// escrita do paciente e gera o registro de auditoria CRIACAO.
var RegisterMedicalRecord = tx.Transaction{
	Tag:         "registerMedicalRecord",
	Label:       "Registrar prontuário",
	Description: "Associa ao paciente o CID de um prontuário cifrado no IPFS",
	Method:      "POST",
	Callers:     institutionCallers,

	Args: []tx.Argument{
		{
			Tag:         "cid",
			Label:       "CID",
			Description: "CID devolvido pelo IPFS",
			DataType:    "string",
			Required:    true,
		},
		{
			Tag:         "contentHash",
			Label:       "Hash do conteúdo",
			Description: "SHA-256 do envelope cifrado",
			DataType:    "sha256",
			Required:    true,
		},
		{
			Tag:      "patient",
			Label:    "Paciente",
			DataType: "->patient",
			Required: true,
		},
		{
			Tag:      "author",
			Label:    "Autor",
			DataType: "->professional",
			Required: true,
		},
		{
			Tag:      "recordType",
			Label:    "Tipo",
			DataType: "recordType",
			Required: true,
		},
	},
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		cid, _ := req["cid"].(string)
		patientKey, _ := req["patient"].(assets.Key)
		authorKey, _ := req["author"].(assets.Key)
		if cid == "" {
			return nil, errors.NewCCError("o CID é obrigatório", 400)
		}

		author, inst, msp, err := callerProfessional(stub, authorKey)
		if err != nil {
			return nil, err
		}
		patient, err := activePatient(stub, patientKey)
		if err != nil {
			return nil, err
		}
		now, err := txTime(stub)
		if err != nil {
			return nil, err
		}

		perm, err := findPermission(stub, patient, author, nil, true, now)
		if err != nil {
			return nil, err
		}
		if perm == nil {
			return nil, errors.NewCCError("o paciente não concedeu permissão de escrita ao autor", 403)
		}

		record, err := assets.NewAsset(map[string]interface{}{
			"@assetType":  "medicalRecord",
			"cid":         cid,
			"contentHash": req["contentHash"],
			"patient":     ref(patient),
			"author":      ref(author),
			"institution": ref(inst),
			"recordType":  req["recordType"],
			"kmsKeyId":    (*patient)["kmsKeyId"],
			"createdAt":   formatTime(now),
			"invalidated": false,
		})
		if err != nil {
			return nil, errors.WrapError(err, "falha ao montar o prontuário")
		}
		exists, err := record.ExistsInLedger(stub)
		if err != nil {
			return nil, errors.WrapError(err, "falha ao verificar o prontuário")
		}
		if exists {
			return nil, errors.NewCCError("CID já registrado", 409)
		}
		if _, err = record.Put(stub); err != nil {
			return nil, errors.WrapError(err, "falha ao gravar o prontuário")
		}

		logAsset, err := writeAudit(stub, inst, msp, auditEntry{
			Operation:  datatypes.OpCreate,
			Patient:    patient,
			Record:     &record,
			Actor:      author,
			Permission: perm,
		})
		if err != nil {
			return nil, err
		}

		resJSON, nerr := json.Marshal(map[string]interface{}{
			"record":    record,
			"accessLog": logAsset.Key(),
		})
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
