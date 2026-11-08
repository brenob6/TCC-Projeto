package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"poc-key-sharing/kmsclient"

	"api-gateway/internal/config"
	"api-gateway/internal/directory"
	"api-gateway/internal/fabric"
)

// fakeLedger responde às transações com funções definidas por teste.
type fakeLedger struct {
	handlers map[string]func(args map[string]any) (any, error)
	calls    []string
}

func (f *fakeLedger) call(fn string, args any) (json.RawMessage, error) {
	f.calls = append(f.calls, fn)
	h, ok := f.handlers[fn]
	if !ok {
		return nil, &fabric.Error{Status: 500, Message: "transação inesperada: " + fn}
	}
	raw, _ := json.Marshal(args)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	out, err := h(m)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (f *fakeLedger) Evaluate(_, fn string, args, _ any) (json.RawMessage, error) {
	return f.call(fn, args)
}

func (f *fakeLedger) Submit(_, fn string, args, _ any) (*fabric.Result, error) {
	out, err := f.call(fn, args)
	if err != nil {
		return nil, err
	}
	return &fabric.Result{TxID: "tx-" + fn, Payload: out}, nil
}

type fakeStorage struct {
	files    map[string][]byte
	unpinned []string
}

func (s *fakeStorage) Add(_ context.Context, _ string, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	cid := "bafy" + hex.EncodeToString(sum[:8])
	s.files[cid] = data
	return cid, nil
}

func (s *fakeStorage) Cat(_ context.Context, cid string) ([]byte, error) {
	data, ok := s.files[cid]
	if !ok {
		return nil, fmt.Errorf("CID inexistente")
	}
	return data, nil
}

func (s *fakeStorage) Unpin(_ context.Context, cid string) error {
	s.unpinned = append(s.unpinned, cid)
	return nil
}

func (s *fakeStorage) ID(context.Context) (string, error) { return "peer", nil }

// fakeKMS não cifra de fato; registra a auditoria recebida na decifração.
type fakeKMS struct {
	decrypts []kmsclient.AuditInfo
}

func (k *fakeKMS) GenerateAndDistribute(context.Context, string) ([]byte, error) {
	return []byte("pub"), nil
}

func (k *fakeKMS) Encrypt(_, plaintext []byte) (*kmsclient.EncryptedRecord, error) {
	return &kmsclient.EncryptedRecord{C1: []byte("c1"), Ciphertext: plaintext}, nil
}

func (k *fakeKMS) Decrypt(_ context.Context, _ string, rec *kmsclient.EncryptedRecord, audit kmsclient.AuditInfo) ([]byte, error) {
	k.decrypts = append(k.decrypts, audit)
	return rec.Ciphertext, nil
}

func (k *fakeKMS) HealthCheck(context.Context) map[string]string { return nil }
func (k *fakeKMS) Threshold() int                                { return 2 }
func (k *fakeKMS) Total() int                                    { return 3 }

type env struct {
	ledger  *fakeLedger
	storage *fakeStorage
	kms     *fakeKMS
	handler http.Handler
}

func newEnv(t *testing.T) *env {
	t.Helper()
	cfg := config.Default(t.TempDir())
	dir, err := directory.Open(filepath.Join(t.TempDir(), "directory.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := dir.PutInstitution(directory.Institution{Org: "Org1", Key: "institution:a", ID: "2345678", Name: "Hospital A"}); err != nil {
		t.Fatal(err)
	}
	e := &env{
		ledger:  &fakeLedger{handlers: map[string]func(map[string]any) (any, error){}},
		storage: &fakeStorage{files: map[string][]byte{}},
		kms:     &fakeKMS{},
	}
	storages := map[string]Storage{}
	for _, o := range cfg.Orgs {
		storages[o.Name] = e.storage
	}
	e.handler = New(cfg, e.ledger, storages, e.kms, dir).Handler()
	return e
}

func (e *env) do(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("X-Org", "Org1")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// withRecordFlow configura o ledger para registrar e autorizar a leitura,
// guardando o hash informado no registro.
func (e *env) withRecordFlow(write, granted bool) {
	var contentHash string
	e.ledger.handlers["checkPermission"] = func(map[string]any) (any, error) {
		return map[string]any{"read": true, "write": write}, nil
	}
	e.ledger.handlers["readAsset"] = func(map[string]any) (any, error) {
		return map[string]any{"publicKey": base64.StdEncoding.EncodeToString([]byte("pub")), "kmsKeyId": "paciente-1"}, nil
	}
	e.ledger.handlers["registerMedicalRecord"] = func(args map[string]any) (any, error) {
		contentHash, _ = args["contentHash"].(string)
		return map[string]any{"record": map[string]any{"@key": "medicalRecord:1", "createdAt": "2026-11-07T12:00:00Z"}}, nil
	}
	e.ledger.handlers["requestRecordAccess"] = func(map[string]any) (any, error) {
		if !granted {
			return map[string]any{"granted": false, "operation": "ACESSO_NEGADO", "accessLog": "accessLog:2", "reason": "sem permissão"}, nil
		}
		return map[string]any{"granted": true, "operation": "LEITURA", "accessLog": "accessLog:1", "contentHash": contentHash, "kmsKeyId": "paciente-1"}, nil
	}
	e.ledger.handlers["getAccessAuthorization"] = func(map[string]any) (any, error) {
		return map[string]any{"valid": true, "emergency": false}, nil
	}
}

func addRecord(t *testing.T, e *env) (int, map[string]any) {
	return e.do(t, http.MethodPost, "/api/records", map[string]any{
		"patient": "patient:1", "author": "professional:1",
		"recordType": "consulta", "title": "Consulta inicial", "content": "PA 12x8",
	})
}

func TestAddRecordRequiresWritePermission(t *testing.T) {
	e := newEnv(t)
	e.withRecordFlow(false, true)

	code, body := addRecord(t, e)
	if code != http.StatusForbidden {
		t.Fatalf("status %d, corpo %v", code, body)
	}
	if len(e.storage.files) != 0 {
		t.Fatal("o envelope não deveria ser gravado no IPFS sem permissão de escrita")
	}
}

func TestAddAndReadRecord(t *testing.T) {
	e := newEnv(t)
	e.withRecordFlow(true, true)

	code, body := addRecord(t, e)
	if code != http.StatusOK {
		t.Fatalf("registro: status %d, corpo %v", code, body)
	}
	cid := body["cid"].(string)

	code, body = e.do(t, http.MethodPost, "/api/records/"+cid+"/access", map[string]any{"professional": "professional:1"})
	if code != http.StatusOK || body["granted"] != true {
		t.Fatalf("leitura: status %d, corpo %v", code, body)
	}
	doc := body["document"].(map[string]any)
	if doc["title"] != "Consulta inicial" || doc["content"] != "PA 12x8" {
		t.Fatalf("documento inesperado: %v", doc)
	}
	if len(e.kms.decrypts) != 1 || e.kms.decrypts[0].InstitutionID != "2345678" {
		t.Fatalf("auditoria enviada ao KMS: %+v", e.kms.decrypts)
	}
}

func TestDeniedReadSkipsStorageAndKMS(t *testing.T) {
	e := newEnv(t)
	e.withRecordFlow(true, false)
	_, body := addRecord(t, e)
	cid := body["cid"].(string)

	code, body := e.do(t, http.MethodPost, "/api/records/"+cid+"/access", map[string]any{"professional": "professional:2"})
	if code != http.StatusOK || body["granted"] != false || body["operation"] != "ACESSO_NEGADO" {
		t.Fatalf("status %d, corpo %v", code, body)
	}
	if len(e.kms.decrypts) != 0 {
		t.Fatal("o KMS não deveria ser acionado em leitura negada")
	}
}

func TestTamperedEnvelopeIsRejected(t *testing.T) {
	e := newEnv(t)
	e.withRecordFlow(true, true)
	_, body := addRecord(t, e)
	cid := body["cid"].(string)
	e.storage.files[cid] = append(e.storage.files[cid], ' ')

	code, _ := e.do(t, http.MethodPost, "/api/records/"+cid+"/access", map[string]any{"professional": "professional:1"})
	if code != http.StatusBadGateway {
		t.Fatalf("status %d, esperado 502", code)
	}
	if len(e.kms.decrypts) != 0 {
		t.Fatal("envelope adulterado não pode chegar ao KMS")
	}
}

func TestChaincodeRejectionUnpinsEnvelope(t *testing.T) {
	e := newEnv(t)
	e.withRecordFlow(true, true)
	e.ledger.handlers["registerMedicalRecord"] = func(map[string]any) (any, error) {
		return nil, &fabric.Error{Status: 409, Message: "CID já registrado"}
	}

	code, body := addRecord(t, e)
	if code != http.StatusConflict || body["error"] != "CID já registrado" {
		t.Fatalf("status %d, corpo %v", code, body)
	}
	if len(e.storage.unpinned) != 1 {
		t.Fatal("o envelope deveria ser removido do IPFS após a recusa")
	}
}

func TestMissingOrgHeader(t *testing.T) {
	e := newEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/patients/patient:1", nil)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, esperado 400", rec.Code)
	}
}

func TestGrantPermissionForwardsSignedFields(t *testing.T) {
	e := newEnv(t)
	var got map[string]any
	e.ledger.handlers["grantPermission"] = func(args map[string]any) (any, error) {
		got = args
		return map[string]any{"permission": map[string]any{"status": "ativa"}}, nil
	}
	code, _ := e.do(t, http.MethodPost, "/api/patients/patient:1/permissions", map[string]any{
		"institution": "institution:b", "accessLevel": "leitura", "purpose": "segunda opinião",
		"records": []string{"bafy1"}, "nonce": 3, "signature": "c2ln",
	})
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if _, ok := got["professional"]; ok {
		t.Fatal("professional não deveria ser enviado quando o beneficiário é a instituição")
	}
	if _, ok := got["expiresAt"]; ok {
		t.Fatal("expiresAt vazio não deveria ser enviado")
	}
	inst := got["institution"].(map[string]any)
	records := got["records"].([]any)
	if inst["@key"] != "institution:b" || records[0].(map[string]any)["cid"] != "bafy1" || got["nonce"] != float64(3) {
		t.Fatalf("argumentos inesperados: %v", got)
	}
}

func TestRegisterPatientGeneratesKMSKey(t *testing.T) {
	e := newEnv(t)
	var got map[string]any
	e.ledger.handlers["getPatientByCpf"] = func(map[string]any) (any, error) {
		return nil, &fabric.Error{Status: 404, Message: "paciente não encontrado"}
	}
	e.ledger.handlers["registerPatient"] = func(args map[string]any) (any, error) {
		got = args
		return map[string]any{"@key": "patient:1", "id": "1"}, nil
	}
	code, body := e.do(t, http.MethodPost, "/api/patients", map[string]any{
		"name": "Maria", "cpf": "529.982.247-25", "signingPublicKey": "cHVi",
	})
	if code != http.StatusOK {
		t.Fatalf("status %d, corpo %v", code, body)
	}
	if got["publicKey"] != base64.StdEncoding.EncodeToString([]byte("pub")) || got["signingPublicKey"] != "cHVi" {
		t.Fatalf("argumentos inesperados: %v", got)
	}
	if _, ok := got["cpf"]; ok {
		t.Fatal("o CPF não pode seguir nos argumentos públicos")
	}
}

func TestRegisterPatientStopsWhenLedgerUnavailable(t *testing.T) {
	e := newEnv(t)
	e.ledger.handlers["getPatientByCpf"] = func(map[string]any) (any, error) {
		return nil, &fabric.Error{Status: 502, Message: "connection refused"}
	}
	code, _ := e.do(t, http.MethodPost, "/api/patients", map[string]any{
		"name": "Maria", "cpf": "52998224725", "signingPublicKey": "cHVi",
	})
	if code != http.StatusBadGateway {
		t.Fatalf("status %d, esperado 502", code)
	}
	for _, c := range e.ledger.calls {
		if c == "registerPatient" {
			t.Fatal("registerPatient não deveria ser chamado")
		}
	}
}
