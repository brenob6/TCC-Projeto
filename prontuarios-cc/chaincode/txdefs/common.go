// Package txdefs implementa as transações do chaincode de prontuários,
// agrupadas pelos itens de backlog do TCC: registro de usuários (B-02),
// prontuários (B-03), controle de acesso (B-04) e recuperação
// extraordinária (B-05).
package txdefs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger-labs/cc-tools/accesscontrol"
	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"
)

// institutionCallers restringe as transações de negócio às organizações das
// instituições de saúde (Org1MSP, Org2MSP, ...).
var institutionCallers = []accesscontrol.Caller{
	{MSP: `$^Org\d+MSP$`},
}

// idNamespace é o espaço de nomes dos UUIDs determinísticos gerados pelo
// chaincode. Todos os endossantes calculam o mesmo valor.
var idNamespace = uuid.NewSHA1(uuid.NameSpaceOID, []byte("prontuarios-cc"))

// mspIndex é o índice de chave composta MSP -> instituição.
const mspIndex = "msp~institution"

var cpfPepper []byte

// SetCPFPepper define o segredo do consórcio usado no HMAC do CPF. O valor
// precisa ser idêntico em todos os peers, ou os endossos divergem.
func SetCPFPepper(pepper string) error {
	if len(pepper) < 16 {
		return fmt.Errorf("o segredo do hash de CPF deve ter ao menos 16 caracteres")
	}
	cpfPepper = []byte(pepper)
	return nil
}

// hashCPF calcula HMAC-SHA256(pepper, cpf). Sem o segredo, o hash não pode ser
// revertido por enumeração dos CPFs possíveis.
func hashCPF(cpf string) (string, errors.ICCError) {
	if len(cpfPepper) == 0 {
		return "", errors.NewCCError("segredo do hash de CPF não configurado", 500)
	}
	mac := hmac.New(sha256.New, cpfPepper)
	mac.Write([]byte(cpf))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func deterministicID(parts ...string) string {
	seed := ""
	for _, p := range parts {
		seed += p + "|"
	}
	return uuid.NewSHA1(idNamespace, []byte(seed)).String()
}

// txTime devolve o timestamp da transação, igual em todos os endossantes.
func txTime(stub *sw.StubWrapper) (time.Time, errors.ICCError) {
	ts, err := stub.Stub.GetTxTimestamp()
	if err != nil {
		return time.Time{}, errors.WrapErrorWithStatus(err, "falha ao obter o timestamp da transação", 500)
	}
	return ts.AsTime().UTC(), nil
}

// formatTime preserva as frações de segundo, que ordenam operações ocorridas
// no mesmo segundo.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// parseTime interpreta uma data lida do ledger (string RFC3339) ou recebida
// como argumento (time.Time).
func parseTime(v interface{}) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		parsed, err := time.Parse(time.RFC3339, t)
		if err != nil {
			return time.Time{}, false
		}
		return parsed, true
	}
	return time.Time{}, false
}

func toInt(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

// ref cria a referência gravada nas propriedades do tipo ->assetType.
func ref(a *assets.Asset) map[string]interface{} {
	return map[string]interface{}{
		"@assetType": a.TypeTag(),
		"@key":       a.Key(),
	}
}

// refKey extrai a chave de uma referência lida do ledger.
func refKey(v interface{}) string {
	switch r := v.(type) {
	case map[string]interface{}:
		key, _ := r["@key"].(string)
		return key
	case assets.Key:
		return r.Key()
	case assets.Asset:
		return r.Key()
	}
	return ""
}

func getAsset(stub *sw.StubWrapper, key assets.Key, label string) (*assets.Asset, errors.ICCError) {
	asset, err := key.Get(stub)
	if err != nil {
		return nil, errors.WrapErrorWithStatus(err, fmt.Sprintf("%s não encontrado", label), 404)
	}
	return asset, nil
}

func getAssetByKey(stub *sw.StubWrapper, assetType, key string, label string) (*assets.Asset, errors.ICCError) {
	return getAsset(stub, assets.Key{"@assetType": assetType, "@key": key}, label)
}

// referrers devolve os ativos do tipo assetType que referenciam key, usando o
// índice de referências mantido pelo cc-tools. Funciona com LevelDB.
func referrers(stub *sw.StubWrapper, key assets.Key, assetType string) ([]*assets.Asset, errors.ICCError) {
	keys, err := key.Referrers(stub, assetType)
	if err != nil {
		return nil, errors.WrapError(err, "falha ao consultar o índice de referências")
	}
	result := make([]*assets.Asset, 0, len(keys))
	for _, k := range keys {
		a, err := k.Get(stub)
		if err != nil {
			return nil, errors.WrapError(err, "falha ao ler ativo referenciado")
		}
		result = append(result, a)
	}
	return result, nil
}

func sortByTime(list []*assets.Asset, prop string) {
	sort.SliceStable(list, func(i, j int) bool {
		ti, _ := parseTime((*list[i])[prop])
		tj, _ := parseTime((*list[j])[prop])
		if ti.Equal(tj) {
			return list[i].Key() < list[j].Key()
		}
		return ti.Before(tj)
	})
}

// callerInstitution devolve a instituição ativa da organização que assinou a
// transação.
func callerInstitution(stub *sw.StubWrapper) (*assets.Asset, string, errors.ICCError) {
	msp, err := stub.GetMSPID()
	if err != nil {
		return nil, "", errors.WrapErrorWithStatus(err, "falha ao identificar o chamador", 500)
	}
	indexKey, err := stub.CreateCompositeKey(mspIndex, []string{msp})
	if err != nil {
		return nil, "", errors.WrapError(err, "falha ao montar o índice de MSP")
	}
	instKey, err := stub.GetState(indexKey)
	if err != nil {
		return nil, "", errors.WrapError(err, "falha ao consultar o índice de MSP")
	}
	if len(instKey) == 0 {
		return nil, "", errors.NewCCError(fmt.Sprintf("nenhuma instituição cadastrada para %s", msp), 403)
	}
	inst, err := getAssetByKey(stub, "institution", string(instKey), "instituição")
	if err != nil {
		return nil, "", err
	}
	if active, _ := (*inst)["active"].(bool); !active {
		return nil, "", errors.NewCCError("a instituição do chamador está inativa", 403)
	}
	return inst, msp, nil
}

// callerProfessional aplica a validação de chamador: o profissional deve estar
// ativo e vinculado à instituição da organização que assinou a transação.
func callerProfessional(stub *sw.StubWrapper, key assets.Key) (prof, inst *assets.Asset, msp string, err errors.ICCError) {
	inst, msp, err = callerInstitution(stub)
	if err != nil {
		return
	}
	prof, err = getAsset(stub, key, "profissional")
	if err != nil {
		return
	}
	if refKey((*prof)["institution"]) != inst.Key() {
		err = errors.NewCCError("o profissional não pertence à instituição do chamador", 403)
		return
	}
	if active, _ := (*prof)["active"].(bool); !active {
		err = errors.NewCCError("o profissional está inativo", 403)
	}
	return
}

func activePatient(stub *sw.StubWrapper, key assets.Key) (*assets.Asset, errors.ICCError) {
	patient, err := getAsset(stub, key, "paciente")
	if err != nil {
		return nil, err
	}
	if active, _ := (*patient)["active"].(bool); !active {
		return nil, errors.NewCCError("o paciente está inativo", 403)
	}
	return patient, nil
}

// auditEntry reúne os campos de um registro de auditoria.
type auditEntry struct {
	Operation  string
	Patient    *assets.Asset
	Record     *assets.Asset
	Actor      *assets.Asset
	Emergency  *assets.Asset
	Permission *assets.Asset
	Details    string
}

// writeAudit grava um accessLog identificado pelo ID da transação.
func writeAudit(stub *sw.StubWrapper, inst *assets.Asset, msp string, e auditEntry) (*assets.Asset, errors.ICCError) {
	now, err := txTime(stub)
	if err != nil {
		return nil, err
	}
	logMap := map[string]interface{}{
		"@assetType":  "accessLog",
		"id":          stub.Stub.GetTxID(),
		"operation":   e.Operation,
		"patient":     ref(e.Patient),
		"institution": ref(inst),
		"callerMsp":   msp,
		"occurredAt":  formatTime(now),
	}
	optional := map[string]*assets.Asset{
		"record":     e.Record,
		"actor":      e.Actor,
		"emergency":  e.Emergency,
		"permission": e.Permission,
	}
	for tag, a := range optional {
		if a != nil {
			logMap[tag] = ref(a)
		}
	}
	if e.Details != "" {
		logMap["details"] = e.Details
	}

	logAsset, err := assets.NewAsset(logMap)
	if err != nil {
		return nil, errors.WrapError(err, "falha ao montar o registro de auditoria")
	}
	if _, err = logAsset.PutNew(stub); err != nil {
		return nil, errors.WrapError(err, "falha ao gravar o registro de auditoria")
	}
	return &logAsset, nil
}

// getAssetByKeyProps busca um ativo a partir das suas propriedades-chave.
func getAssetByKeyProps(stub *sw.StubWrapper, assetType string, keyProps map[string]interface{}, label string) (*assets.Asset, errors.ICCError) {
	m := map[string]interface{}{"@assetType": assetType}
	for k, v := range keyProps {
		m[k] = v
	}
	key, err := assets.NewKey(m)
	if err != nil {
		return nil, errors.WrapErrorWithStatus(err, "chave inválida", 400)
	}
	return getAsset(stub, key, label)
}
