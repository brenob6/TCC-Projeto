package txdefs

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
	tx "github.com/hyperledger-labs/cc-tools/transactions"

	"prontuarios-cc/datatypes"
)

// GrantPermission registra o consentimento do paciente (RF-04). A mensagem
// assinada contém o beneficiário, o nível de acesso, a finalidade, o escopo e a
// validade, de modo que a API não pode alterar nenhum desses campos.
var GrantPermission = tx.Transaction{
	Tag:         "grantPermission",
	Label:       "Conceder permissão",
	Description: "Concede a um profissional ou a uma instituição acesso aos prontuários do paciente",
	Method:      "POST",
	Callers:     institutionCallers,

	Args: append([]tx.Argument{
		{
			Tag:      "patient",
			Label:    "Paciente",
			DataType: "->patient",
			Required: true,
		},
		{
			Tag:         "professional",
			Label:       "Profissional",
			Description: "Informe o profissional ou a instituição, nunca ambos",
			DataType:    "->professional",
		},
		{
			Tag:      "institution",
			Label:    "Instituição",
			DataType: "->institution",
		},
		{
			Tag:      "accessLevel",
			Label:    "Nível de acesso",
			DataType: "accessLevel",
			Required: true,
		},
		{
			Tag:      "purpose",
			Label:    "Finalidade",
			DataType: "string",
			Required: true,
		},
		{
			Tag:         "records",
			Label:       "Prontuários",
			Description: "Escopo restrito (somente leitura); vazio indica todos",
			DataType:    "[]->medicalRecord",
		},
		{
			Tag:      "expiresAt",
			Label:    "Válida até",
			DataType: "datetime",
		},
	}, signatureArgs...),
	Routine: func(stub *sw.StubWrapper, req map[string]interface{}) ([]byte, errors.ICCError) {
		patientKey, _ := req["patient"].(assets.Key)
		profKey, hasProf := req["professional"].(assets.Key)
		instKey, hasInst := req["institution"].(assets.Key)
		level, _ := req["accessLevel"].(string)
		purpose, _ := req["purpose"].(string)
		recordKeys, _ := req["records"].([]interface{})
		expiresAt, hasExpiry := req["expiresAt"].(time.Time)

		if hasProf == hasInst {
			return nil, errors.NewCCError("informe exatamente um beneficiário: professional ou institution", 400)
		}
		if purpose == "" {
			return nil, errors.NewCCError("a finalidade é obrigatória", 400)
		}
		if level == datatypes.AccessReadWrite && len(recordKeys) > 0 {
			return nil, errors.NewCCError("permissões de escrita não aceitam escopo restrito", 400)
		}

		callerInst, msp, err := callerInstitution(stub)
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
		if hasExpiry && !expiresAt.After(now) {
			return nil, errors.NewCCError("a validade deve ser uma data futura", 400)
		}

		permMap := map[string]interface{}{
			"@assetType":  "accessPermission",
			"id":          stub.Stub.GetTxID(),
			"patient":     ref(patient),
			"accessLevel": level,
			"purpose":     purpose,
			"grantedAt":   formatTime(now),
			"status":      datatypes.PermissionActive,
		}
		payload := map[string]interface{}{
			"professional": "",
			"institution":  "",
			"accessLevel":  level,
			"purpose":      purpose,
			"expiresAt":    "",
		}

		if hasProf {
			prof, err := getAsset(stub, profKey, "profissional")
			if err != nil {
				return nil, err
			}
			permMap["professional"] = ref(prof)
			payload["professional"] = (*prof)["id"]
		} else {
			inst, err := getAsset(stub, instKey, "instituição")
			if err != nil {
				return nil, err
			}
			permMap["institution"] = ref(inst)
			payload["institution"] = (*inst)["id"]
		}

		cids := []string{}
		refs := []interface{}{}
		for _, rk := range recordKeys {
			key, _ := rk.(assets.Key)
			record, err := recordOfPatient(stub, key, patient)
			if err != nil {
				return nil, err
			}
			if isInvalidated(record) {
				return nil, errors.NewCCError("o escopo contém prontuário eliminado", 400)
			}
			cids = append(cids, (*record)["cid"].(string))
			refs = append(refs, ref(record))
		}
		sort.Strings(cids)
		payload["records"] = cids
		if len(refs) > 0 {
			permMap["records"] = refs
		}
		if hasExpiry {
			permMap["expiresAt"] = formatTime(expiresAt)
			payload["expiresAt"] = formatTime(expiresAt)
		}

		signature, err := verifyPatientSignature(stub, patient, "grantPermission", req, payload)
		if err != nil {
			return nil, err
		}
		permMap["patientSignature"] = signature

		perm, err := assets.NewAsset(permMap)
		if err != nil {
			return nil, errors.WrapError(err, "falha ao montar a permissão")
		}
		if _, err = perm.PutNew(stub); err != nil {
			return nil, errors.WrapError(err, "falha ao gravar a permissão")
		}

		logAsset, err := writeAudit(stub, callerInst, msp, auditEntry{
			Operation:  datatypes.OpGrant,
			Patient:    patient,
			Permission: &perm,
			Details:    purpose,
		})
		if err != nil {
			return nil, err
		}

		resJSON, nerr := json.Marshal(map[string]interface{}{
			"permission": perm,
			"accessLog":  logAsset.Key(),
		})
		if nerr != nil {
			return nil, errors.WrapError(nerr, "falha ao serializar a resposta")
		}
		return resJSON, nil
	},
}
