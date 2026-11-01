package txdefs

import (
	"time"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/errors"
	sw "github.com/hyperledger-labs/cc-tools/stubwrapper"

	"prontuarios-cc/datatypes"
)

// EmergencyWindow é o prazo de uso de um acesso extraordinário.
var EmergencyWindow = time.Hour

// MinEmergencyReasonLength é o tamanho mínimo da justificativa emergencial.
const MinEmergencyReasonLength = 10

// permissionStatusAt devolve a situação efetiva da permissão no instante now.
func permissionStatusAt(perm *assets.Asset, now time.Time) string {
	status, _ := (*perm)["status"].(string)
	if status != datatypes.PermissionActive {
		return status
	}
	if exp, ok := parseTime((*perm)["expiresAt"]); ok && !now.Before(exp) {
		return datatypes.PermissionExpired
	}
	return status
}

// emergencyStatusAt devolve a situação efetiva do acesso extraordinário.
func emergencyStatusAt(em *assets.Asset, now time.Time) string {
	status, _ := (*em)["status"].(string)
	if status != datatypes.EmergencyActive {
		return status
	}
	if exp, ok := parseTime((*em)["expiresAt"]); ok && !now.Before(exp) {
		return datatypes.EmergencyExpired
	}
	return status
}

// permissionCovers indica se a permissão autoriza o profissional a ler o
// prontuário record ou, com record nulo e needWrite, a registrar um novo
// prontuário. Permissões de escrita não têm escopo restrito.
func permissionCovers(perm, prof, record *assets.Asset, needWrite bool, now time.Time) bool {
	if permissionStatusAt(perm, now) != datatypes.PermissionActive {
		return false
	}

	grantee := refKey((*perm)["professional"]) == prof.Key() ||
		(refKey((*perm)["institution"]) != "" && refKey((*perm)["institution"]) == refKey((*prof)["institution"]))
	if !grantee {
		return false
	}

	if needWrite && (*perm)["accessLevel"] != datatypes.AccessReadWrite {
		return false
	}

	scope, _ := (*perm)["records"].([]interface{})
	switch {
	case len(scope) == 0:
		return true
	case needWrite:
		return false
	case record == nil:
		// Consulta sem prontuário específico: basta haver algum acesso.
		return true
	}
	for _, r := range scope {
		if refKey(r) == record.Key() {
			return true
		}
	}
	return false
}

// findPermission procura uma permissão do paciente que cubra o acesso.
func findPermission(stub *sw.StubWrapper, patient, prof, record *assets.Asset, needWrite bool, now time.Time) (*assets.Asset, errors.ICCError) {
	perms, err := referrers(stub, assets.Key{"@assetType": "patient", "@key": patient.Key()}, "accessPermission")
	if err != nil {
		return nil, err
	}
	sortByTime(perms, "grantedAt")
	for _, perm := range perms {
		if permissionCovers(perm, prof, record, needWrite, now) {
			return perm, nil
		}
	}
	return nil, nil
}

// findEmergency procura um acesso extraordinário ativo do profissional para o
// prontuário.
func findEmergency(stub *sw.StubWrapper, record, prof *assets.Asset, now time.Time) (*assets.Asset, errors.ICCError) {
	ems, err := referrers(stub, assets.Key{"@assetType": "medicalRecord", "@key": record.Key()}, "emergencyAccess")
	if err != nil {
		return nil, err
	}
	sortByTime(ems, "requestedAt")
	for _, em := range ems {
		if emergencyStatusAt(em, now) == datatypes.EmergencyActive && refKey((*em)["professional"]) == prof.Key() {
			return em, nil
		}
	}
	return nil, nil
}

// recordOfPatient valida que o prontuário pertence ao paciente.
func recordOfPatient(stub *sw.StubWrapper, key assets.Key, patient *assets.Asset) (*assets.Asset, errors.ICCError) {
	record, err := getAsset(stub, key, "prontuário")
	if err != nil {
		return nil, err
	}
	if refKey((*record)["patient"]) != patient.Key() {
		return nil, errors.NewCCError("o prontuário não pertence ao paciente", 400)
	}
	return record, nil
}

func isInvalidated(record *assets.Asset) bool {
	invalidated, _ := (*record)["invalidated"].(bool)
	return invalidated
}

// inPeriod aplica os filtros opcionais from/to sobre a propriedade prop.
func inPeriod(a *assets.Asset, prop string, req map[string]interface{}) bool {
	t, ok := parseTime((*a)[prop])
	if !ok {
		return false
	}
	if from, ok := req["from"].(time.Time); ok && t.Before(from) {
		return false
	}
	if to, ok := req["to"].(time.Time); ok && t.After(to) {
		return false
	}
	return true
}
