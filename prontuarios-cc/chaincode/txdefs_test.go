package main

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"prontuarios-cc/datatypes"
)

// B-02 – Registro de usuários

func TestRegisterInstitution(t *testing.T) {
	f := newFixture(t)

	if f.hospitalA["mspId"] != "Org1MSP" || f.hospitalA["active"] != true {
		t.Fatalf("instituição gravada incorretamente: %v", f.hospitalA)
	}

	f.expectStatus(409, "Org1MSP", "registerInstitution", map[string]interface{}{
		"id": "9999999", "name": "Outra", "cnpj": "11.222.333/0001-81",
	}, nil)
	f.expectStatus(400, "Org4MSP", "registerInstitution", map[string]interface{}{
		"id": "9999999", "name": "Outra", "cnpj": "11.222.333/0001-00",
	}, nil)
	f.expectStatus(403, "OrdererMSP", "registerInstitution", map[string]interface{}{
		"id": "9999999", "name": "Outra", "cnpj": "11.222.333/0001-81",
	}, nil)
}

func TestSetInstitutionStatus(t *testing.T) {
	f := newFixture(t)
	instRef := ref(f.hospitalA["@key"].(string))

	f.expectStatus(403, "Org2MSP", "setInstitutionStatus", map[string]interface{}{"institution": instRef, "active": false}, nil)
	f.mustInvoke("Org1MSP", "setInstitutionStatus", map[string]interface{}{"institution": instRef, "active": false}, nil)

	// Instituição inativa não opera.
	f.expectStatus(403, "Org1MSP", "registerProfessional", map[string]interface{}{
		"name": "Dr. Novo", "professionalType": "medico", "councilRegistry": "CRM-DF 9", "institution": instRef,
	}, map[string]interface{}{"cpf": "390.533.447-05"})

	f.mustInvoke("Org1MSP", "setInstitutionStatus", map[string]interface{}{"institution": instRef, "active": true}, nil)
}

func TestRegisterPatientDoesNotStoreCPF(t *testing.T) {
	f := newFixture(t)

	for key, value := range f.state {
		if bytes.Contains(value, []byte("52998224725")) || strings.Contains(key, "52998224725") {
			t.Fatalf("CPF em claro encontrado no estado (%s)", key)
		}
	}

	found := f.mustInvoke("Org2MSP", "getPatientByCpf", nil, map[string]interface{}{"cpf": cpfPaciente}).object(t)
	if found["id"] != f.patient.id {
		t.Fatalf("getPatientByCpf devolveu %v", found["id"])
	}
	if found["signatureNonce"] != float64(0) {
		t.Fatalf("nonce inicial deveria ser 0: %v", found["signatureNonce"])
	}

	// O CPF deve chegar pelo mapa transiente.
	f.expectStatus(400, "Org1MSP", "getPatientByCpf", map[string]interface{}{"cpf": cpfPaciente}, nil)
	f.expectStatus(409, "Org2MSP", "registerPatient", map[string]interface{}{
		"kmsKeyId": "outro", "publicKey": "eA==", "signingPublicKey": "11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo=",
	}, map[string]interface{}{"cpf": cpfPaciente})
	f.expectStatus(400, "Org1MSP", "registerPatient", map[string]interface{}{
		"kmsKeyId": "outro", "publicKey": "eA==", "signingPublicKey": "11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo=",
	}, map[string]interface{}{"cpf": "111.111.111-11"})
	f.expectStatus(404, "Org1MSP", "getPatientByCpf", nil, map[string]interface{}{"cpf": "390.533.447-05"})
}

func TestRegisterProfessional(t *testing.T) {
	f := newFixture(t)

	if f.medicoA["emergencyAccessAllowed"] != true || f.enfermeiro["emergencyAccessAllowed"] != false {
		t.Fatal("habilitação emergencial padrão incorreta")
	}

	// Org2 não pode vincular profissional ao Hospital A.
	f.expectStatus(403, "Org2MSP", "registerProfessional", map[string]interface{}{
		"name": "Dr. Intruso", "professionalType": "medico", "councilRegistry": "CRM-DF 9",
		"institution": ref(f.hospitalA["@key"].(string)),
	}, map[string]interface{}{"cpf": "390.533.447-05"})

	f.expectStatus(409, "Org1MSP", "registerProfessional", map[string]interface{}{
		"name": "Dra. Ana", "professionalType": "medico", "councilRegistry": "CRM-DF 1001",
		"institution": ref(f.hospitalA["@key"].(string)),
	}, map[string]interface{}{"cpf": cpfMedicoA})

	// O mesmo CPF pode ter vínculo em outra instituição.
	f.registerProfessional("Org2MSP", f.hospitalB, cpfMedicoA, "Dra. Ana", "medico", "CRM-DF 1001")

	f.expectStatus(403, "Org2MSP", "setProfessionalStatus", map[string]interface{}{
		"professional": ref(f.medicoA["@key"].(string)), "active": false,
	}, nil)
	f.mustInvoke("Org1MSP", "setProfessionalStatus", map[string]interface{}{
		"professional": ref(f.medicoA["@key"].(string)), "active": false,
	}, nil)
	f.expectStatus(403, "Org1MSP", "checkPermission", map[string]interface{}{
		"patient": f.patientRef(), "professional": ref(f.medicoA["@key"].(string)),
	}, nil)
}

// B-03 e B-04 – fluxo regular

func TestRegularFlow(t *testing.T) {
	f := newFixture(t)
	medico := map[string]interface{}{"professional": f.medicoA["id"]}

	// Sem consentimento, o registro é recusado (D3).
	if res := f.registerRecord("Org1MSP", f.medicoA, "cid-001"); res.status != 403 {
		t.Fatalf("registro sem consentimento: status %d", res.status)
	}

	// Permissão somente de leitura não autoriza escrita.
	if res := f.grant("Org1MSP", medico, datatypes.AccessRead, nil, nil); res.status != 200 {
		t.Fatal(res.message)
	}
	if res := f.registerRecord("Org1MSP", f.medicoA, "cid-001"); res.status != 403 {
		t.Fatalf("registro com permissão de leitura: status %d", res.status)
	}

	grant := f.grant("Org1MSP", medico, datatypes.AccessReadWrite, nil, nil)
	if grant.status != 200 {
		t.Fatal(grant.message)
	}
	perm := grant.object(t)["permission"].(map[string]interface{})

	created := f.registerRecord("Org1MSP", f.medicoA, "cid-001")
	if created.status != 200 {
		t.Fatal(created.message)
	}
	record := created.object(t)["record"].(map[string]interface{})
	if record["kmsKeyId"] != "paciente-001" || record["institution"].(map[string]interface{})["@key"] != f.hospitalA["@key"] {
		t.Fatalf("prontuário gravado incorretamente: %v", record)
	}
	if res := f.registerRecord("Org1MSP", f.medicoA, "cid-001"); res.status != 409 {
		t.Fatalf("CID duplicado: status %d", res.status)
	}

	read := f.readRecord("Org1MSP", f.medicoA, "cid-001")
	if read["granted"] != true || read["operation"] != datatypes.OpRead || read["kmsKeyId"] != "paciente-001" {
		t.Fatalf("leitura autorizada: %v", read)
	}

	auth := f.mustInvoke("Org2MSP", "getAccessAuthorization", map[string]interface{}{
		"accessLog": ref(read["accessLog"].(string)),
	}, nil).object(t)
	if auth["valid"] != true || auth["cid"] != "cid-001" || auth["kmsKeyId"] != "paciente-001" {
		t.Fatalf("autorização para o KMS: %v", auth)
	}

	// Revogação assinada pelo paciente tem efeito imediato.
	f.mustInvoke("Org1MSP", "revokePermission", merge(
		map[string]interface{}{"permission": ref(perm["@key"].(string))},
		f.patient.sign(t, "revokePermission", map[string]interface{}{"permission": perm["id"]}),
	), nil)

	// Ainda resta a permissão de leitura; revoga-a também.
	perms := f.mustInvoke("Org1MSP", "listPermissions", map[string]interface{}{
		"patient": f.patientRef(), "onlyActive": true,
	}, nil).list(t)
	if len(perms) != 1 || perms[0]["accessLevel"] != datatypes.AccessRead {
		t.Fatalf("permissões ativas: %v", perms)
	}
	f.mustInvoke("Org1MSP", "revokePermission", merge(
		map[string]interface{}{"permission": ref(perms[0]["@key"].(string))},
		f.patient.sign(t, "revokePermission", map[string]interface{}{"permission": perms[0]["id"]}),
	), nil)

	denied := f.readRecord("Org1MSP", f.medicoA, "cid-001")
	if denied["granted"] != false || denied["operation"] != datatypes.OpDenied {
		t.Fatalf("leitura após revogação: %v", denied)
	}
	auth = f.mustInvoke("Org1MSP", "getAccessAuthorization", map[string]interface{}{
		"accessLog": ref(denied["accessLog"].(string)),
	}, nil).object(t)
	if auth["valid"] != false {
		t.Fatalf("negativa não pode autorizar decifração: %v", auth)
	}

	// RF-08: histórico completo, em ordem.
	history := f.mustInvoke("Org3MSP", "getAccessHistory", map[string]interface{}{
		"patient": f.patientRef(),
	}, nil).list(t)
	want := []string{
		datatypes.OpGrant, datatypes.OpGrant, datatypes.OpCreate, datatypes.OpRead,
		datatypes.OpRevoke, datatypes.OpRevoke, datatypes.OpDenied,
	}
	if len(history) != len(want) {
		t.Fatalf("histórico com %d registros, esperado %d", len(history), len(want))
	}
	for i, op := range want {
		if history[i]["operation"] != op {
			t.Fatalf("histórico[%d] = %v, esperado %s", i, history[i]["operation"], op)
		}
	}
	if history[3]["actor"].(map[string]interface{})["@key"] != f.medicoA["@key"] || history[3]["callerMsp"] != "Org1MSP" {
		t.Fatalf("registro de leitura sem responsável ou instituição: %v", history[3])
	}

	byRecord := f.mustInvoke("Org1MSP", "getAccessHistory", map[string]interface{}{
		"patient": f.patientRef(), "record": map[string]interface{}{"cid": "cid-001"},
	}, nil).list(t)
	if len(byRecord) != 3 {
		t.Fatalf("histórico do prontuário com %d registros, esperado 3", len(byRecord))
	}
}

func TestAuthorizationExpires(t *testing.T) {
	f := newFixture(t)
	f.grant("Org1MSP", map[string]interface{}{"professional": f.medicoA["id"]}, datatypes.AccessReadWrite, nil, nil)
	f.registerRecord("Org1MSP", f.medicoA, "cid-001")
	read := f.readRecord("Org1MSP", f.medicoA, "cid-001")

	f.setState(read["accessLog"].(string), "occurredAt", time.Now().Add(-10*time.Minute).UTC().Format(time.RFC3339))
	auth := f.mustInvoke("Org1MSP", "getAccessAuthorization", map[string]interface{}{
		"accessLog": ref(read["accessLog"].(string)),
	}, nil).object(t)
	if auth["valid"] != false {
		t.Fatalf("autorização antiga aceita: %v", auth)
	}
}

func TestCallerValidation(t *testing.T) {
	f := newFixture(t)
	f.grant("Org1MSP", map[string]interface{}{"professional": f.medicoA["id"]}, datatypes.AccessReadWrite, nil, nil)
	f.registerRecord("Org1MSP", f.medicoA, "cid-001")

	// Org2 não pode agir em nome de profissional do Hospital A.
	f.expectStatus(403, "Org2MSP", "requestRecordAccess", map[string]interface{}{
		"record": map[string]interface{}{"cid": "cid-001"}, "professional": ref(f.medicoA["@key"].(string)),
	}, nil)
	if res := f.registerRecord("Org2MSP", f.medicoA, "cid-002"); res.status != 403 {
		t.Fatalf("registro por outra organização: status %d", res.status)
	}
}

func TestInstitutionAndScopedPermissions(t *testing.T) {
	f := newFixture(t)
	f.grant("Org1MSP", map[string]interface{}{"professional": f.medicoA["id"]}, datatypes.AccessReadWrite, nil, nil)
	f.registerRecord("Org1MSP", f.medicoA, "cid-001")
	f.registerRecord("Org1MSP", f.medicoA, "cid-002")

	// Hospital B inteiro, restrito ao cid-001.
	if res := f.grant("Org2MSP", map[string]interface{}{"institution": f.hospitalB["id"]}, datatypes.AccessRead, []string{"cid-001"}, nil); res.status != 200 {
		t.Fatal(res.message)
	}
	if got := f.readRecord("Org2MSP", f.medicoB, "cid-001"); got["granted"] != true {
		t.Fatalf("leitura dentro do escopo: %v", got)
	}
	if got := f.readRecord("Org2MSP", f.medicoB, "cid-002"); got["granted"] != false {
		t.Fatalf("leitura fora do escopo: %v", got)
	}
	// Permissão individual não se estende aos colegas da instituição.
	if got := f.readRecord("Org1MSP", f.enfermeiro, "cid-001"); got["granted"] != false {
		t.Fatalf("leitura por colega sem permissão: %v", got)
	}

	list := f.mustInvoke("Org2MSP", "listPatientRecords", map[string]interface{}{
		"patient": f.patientRef(), "professional": ref(f.medicoB["@key"].(string)),
	}, nil).list(t)
	if len(list) != 1 || list[0]["cid"] != "cid-001" {
		t.Fatalf("listPatientRecords: %v", list)
	}

	check := f.mustInvoke("Org2MSP", "checkPermission", map[string]interface{}{
		"patient": f.patientRef(), "professional": ref(f.medicoB["@key"].(string)),
		"record": map[string]interface{}{"cid": "cid-002"},
	}, nil).object(t)
	if check["read"] != false || check["write"] != false {
		t.Fatalf("checkPermission: %v", check)
	}

	// Escrita não aceita escopo restrito.
	if res := f.grant("Org1MSP", map[string]interface{}{"professional": f.enfermeiro["id"]}, datatypes.AccessReadWrite, []string{"cid-001"}, nil); res.status != 400 {
		t.Fatalf("escrita com escopo: status %d", res.status)
	}
}

func TestPermissionExpiry(t *testing.T) {
	f := newFixture(t)
	expires := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	grant := f.grant("Org1MSP", map[string]interface{}{"professional": f.medicoA["id"]}, datatypes.AccessReadWrite, nil,
		map[string]interface{}{"expiresAt": expires})
	if grant.status != 200 {
		t.Fatal(grant.message)
	}
	permKey := grant.object(t)["permission"].(map[string]interface{})["@key"].(string)
	f.registerRecord("Org1MSP", f.medicoA, "cid-001")

	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if res := f.grant("Org1MSP", map[string]interface{}{"professional": f.medicoA["id"]}, datatypes.AccessRead, nil,
		map[string]interface{}{"expiresAt": past}); res.status != 400 {
		t.Fatalf("validade no passado: status %d", res.status)
	}

	f.setState(permKey, "expiresAt", past)
	if got := f.readRecord("Org1MSP", f.medicoA, "cid-001"); got["granted"] != false {
		t.Fatalf("leitura com permissão vencida: %v", got)
	}
	perms := f.mustInvoke("Org1MSP", "listPermissions", map[string]interface{}{"patient": f.patientRef()}, nil).list(t)
	if len(perms) != 1 || perms[0]["status"] != datatypes.PermissionExpired {
		t.Fatalf("listPermissions: %v", perms)
	}
}

// D2 – Assinatura do paciente

func TestPatientSignature(t *testing.T) {
	f := newFixture(t)
	medico := map[string]interface{}{"professional": f.medicoA["id"]}

	// Assinatura de outro conteúdo (finalidade adulterada pela API).
	payload := map[string]interface{}{
		"professional": f.medicoA["id"], "institution": "", "accessLevel": datatypes.AccessReadWrite,
		"purpose": "pesquisa", "records": []string{}, "expiresAt": "",
	}
	args := map[string]interface{}{
		"patient": f.patientRef(), "professional": map[string]interface{}{"id": f.medicoA["id"]},
		"accessLevel": datatypes.AccessReadWrite, "purpose": "acompanhamento clínico",
	}
	f.expectStatus(401, "Org1MSP", "grantPermission", merge(args, f.patient.sign(t, "grantPermission", payload)), nil)
	f.patient.nonce-- // a transação falhou; o nonce não foi consumido no ledger real

	// Assinatura de outra chave.
	other := &patientWallet{id: f.patient.id, nonce: f.patient.nonce}
	_, other.priv, _ = ed25519GenerateKey()
	payload["purpose"] = "acompanhamento clínico"
	f.expectStatus(401, "Org1MSP", "grantPermission", merge(args, other.sign(t, "grantPermission", payload)), nil)

	// Assinatura válida.
	signed := merge(args, f.patient.sign(t, "grantPermission", payload))
	f.mustInvoke("Org1MSP", "grantPermission", signed, nil)

	// Repetição da mesma mensagem.
	f.expectStatus(409, "Org1MSP", "grantPermission", signed, nil)

	// A permissão guarda a assinatura.
	perms := f.mustInvoke("Org1MSP", "listPermissions", map[string]interface{}{"patient": f.patientRef()}, nil).list(t)
	if len(perms) != 1 || perms[0]["patientSignature"] != signed["signature"] {
		t.Fatalf("assinatura não registrada: %v", perms)
	}

	// Exatamente um beneficiário.
	both := merge(medico, map[string]interface{}{"institution": f.hospitalA["id"]})
	if res := f.grant("Org1MSP", both, datatypes.AccessRead, nil, nil); res.status != 400 {
		t.Fatalf("dois beneficiários: status %d", res.status)
	}
}

func TestInvalidateMedicalRecord(t *testing.T) {
	f := newFixture(t)
	f.grant("Org1MSP", map[string]interface{}{"professional": f.medicoA["id"]}, datatypes.AccessReadWrite, nil, nil)
	f.registerRecord("Org1MSP", f.medicoA, "cid-001")

	invalidate := map[string]interface{}{"record": map[string]interface{}{"cid": "cid-001"}, "reason": "pedido do titular"}
	f.mustInvoke("Org2MSP", "invalidateMedicalRecord", merge(invalidate,
		f.patient.sign(t, "invalidateMedicalRecord", map[string]interface{}{"record": "cid-001", "reason": "pedido do titular"})), nil)

	if got := f.readRecord("Org1MSP", f.medicoA, "cid-001"); got["granted"] != false {
		t.Fatalf("leitura de prontuário eliminado: %v", got)
	}
	list := f.mustInvoke("Org1MSP", "listPatientRecords", map[string]interface{}{
		"patient": f.patientRef(), "professional": ref(f.medicoA["@key"].(string)),
	}, nil).list(t)
	if len(list) != 0 {
		t.Fatalf("prontuário eliminado listado: %v", list)
	}
	f.expectStatus(409, "Org2MSP", "invalidateMedicalRecord", merge(invalidate,
		f.patient.sign(t, "invalidateMedicalRecord", map[string]interface{}{"record": "cid-001", "reason": "pedido do titular"})), nil)
}

// B-05 e D4 – Recuperação extraordinária

func TestEmergencyAccess(t *testing.T) {
	f := newFixture(t)
	f.grant("Org1MSP", map[string]interface{}{"professional": f.medicoA["id"]}, datatypes.AccessReadWrite, nil, nil)
	f.registerRecord("Org1MSP", f.medicoA, "cid-001")

	request := map[string]interface{}{
		"patient":      f.patientRef(),
		"professional": ref(f.medicoB["@key"].(string)),
		"record":       map[string]interface{}{"cid": "cid-001"},
		"reason":       "paciente inconsciente no pronto-socorro",
	}

	// Sem permissão, o médico do Hospital B é negado.
	if got := f.readRecord("Org2MSP", f.medicoB, "cid-001"); got["granted"] != false {
		t.Fatalf("leitura sem permissão: %v", got)
	}

	f.expectStatus(400, "Org2MSP", "requestEmergencyAccess", merge(request, map[string]interface{}{"reason": "urgente"}), nil)
	f.expectStatus(403, "Org1MSP", "requestEmergencyAccess", merge(request, map[string]interface{}{
		"professional": ref(f.enfermeiro["@key"].(string)),
	}), nil)

	em := f.mustInvoke("Org2MSP", "requestEmergencyAccess", request, nil).object(t)["emergencyAccess"].(map[string]interface{})
	requested, _ := time.Parse(time.RFC3339, em["requestedAt"].(string))
	expires, _ := time.Parse(time.RFC3339, em["expiresAt"].(string))
	if expires.Sub(requested) != time.Hour || em["status"] != datatypes.EmergencyActive {
		t.Fatalf("acesso extraordinário: %v", em)
	}

	read := f.readRecord("Org2MSP", f.medicoB, "cid-001")
	if read["granted"] != true || read["operation"] != datatypes.OpEmergencyRead {
		t.Fatalf("leitura emergencial: %v", read)
	}
	auth := f.mustInvoke("Org2MSP", "getAccessAuthorization", map[string]interface{}{
		"accessLog": ref(read["accessLog"].(string)),
	}, nil).object(t)
	if auth["valid"] != true || auth["emergency"] != true {
		t.Fatalf("autorização emergencial: %v", auth)
	}

	// Uso único.
	if got := f.readRecord("Org2MSP", f.medicoB, "cid-001"); got["granted"] != false {
		t.Fatalf("segunda leitura emergencial: %v", got)
	}

	ems := f.mustInvoke("Org1MSP", "listEmergencyAccesses", map[string]interface{}{"patient": f.patientRef()}, nil).list(t)
	if len(ems) != 1 || ems[0]["status"] != datatypes.EmergencyUsed || ems[0]["reason"] != request["reason"] {
		t.Fatalf("listEmergencyAccesses: %v", ems)
	}

	history := f.mustInvoke("Org1MSP", "getAccessHistory", map[string]interface{}{
		"patient": f.patientRef(), "record": map[string]interface{}{"cid": "cid-001"},
	}, nil).list(t)
	ops := []string{}
	for _, h := range history {
		ops = append(ops, h["operation"].(string))
	}
	want := strings.Join([]string{
		datatypes.OpCreate, datatypes.OpDenied, datatypes.OpEmergencyRequest, datatypes.OpEmergencyRead, datatypes.OpDenied,
	}, ",")
	if strings.Join(ops, ",") != want {
		t.Fatalf("histórico emergencial: %s, esperado %s", strings.Join(ops, ","), want)
	}
}

func TestEmergencyAccessExpires(t *testing.T) {
	f := newFixture(t)
	f.grant("Org1MSP", map[string]interface{}{"professional": f.medicoA["id"]}, datatypes.AccessReadWrite, nil, nil)
	f.registerRecord("Org1MSP", f.medicoA, "cid-001")

	em := f.mustInvoke("Org2MSP", "requestEmergencyAccess", map[string]interface{}{
		"patient":      f.patientRef(),
		"professional": ref(f.medicoB["@key"].(string)),
		"record":       map[string]interface{}{"cid": "cid-001"},
		"reason":       "paciente inconsciente no pronto-socorro",
	}, nil).object(t)["emergencyAccess"].(map[string]interface{})

	f.setState(em["@key"].(string), "expiresAt", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
	if got := f.readRecord("Org2MSP", f.medicoB, "cid-001"); got["granted"] != false {
		t.Fatalf("leitura com acesso extraordinário vencido: %v", got)
	}
	ems := f.mustInvoke("Org2MSP", "listEmergencyAccesses", map[string]interface{}{"patient": f.patientRef()}, nil).list(t)
	if ems[0]["status"] != datatypes.EmergencyExpired {
		t.Fatalf("situação calculada: %v", ems[0]["status"])
	}
}

func TestGenericWriteTransactionsDisabled(t *testing.T) {
	f := newFixture(t)
	for _, fn := range []string{"createAsset", "updateAsset", "deleteAsset", "createAssetType"} {
		f.expectStatus(400, "Org1MSP", fn, map[string]interface{}{}, nil)
	}
}

func ed25519GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(nil)
}
