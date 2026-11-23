package ledger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Os caminhos relativos são resolvidos a partir do diretório da configuração.
func TestLoadResolveCaminhos(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.json")
	if err := os.WriteFile(path, []byte(`{
		"mspId": "Org1MSP", "peerAddress": "192.168.56.31:7051", "peerHost": "peer0.org1.example.com",
		"tlsCaCert": "tls-ca.crt", "certPath": "/abs/cert.pem", "keyPath": "key.pem"
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLSCACert != filepath.Join(dir, "tls-ca.crt") || cfg.CertPath != "/abs/cert.pem" ||
		cfg.KeyPath != filepath.Join(dir, "key.pem") {
		t.Fatalf("caminhos: %+v", cfg)
	}
	if cfg.Channel != "prontuarios" || cfg.Chaincode != "prontuarios" {
		t.Fatalf("canal e chaincode padrão: %+v", cfg)
	}

	if err := os.WriteFile(path, []byte(`{"peerAddress": "x:1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("configuração sem mspId deveria ser recusada")
	}
}

// A resposta de getAccessAuthorization é convertida sem perda de campos.
func TestParseResposta(t *testing.T) {
	a, err := parse([]byte(`{"valid": true, "kmsKeyId": "paciente-1", "contentHash": "abc",
		"actor": "professional:1", "institution": "institution:1", "emergency": true,
		"emergencyReason": "paciente inconsciente", "cid": "bafy"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !a.Valid || a.KMSKeyID != "paciente-1" || a.ContentHash != "abc" || a.Actor != "professional:1" ||
		a.Institution != "institution:1" || !a.Emergency || a.EmergencyReason != "paciente inconsciente" {
		t.Fatalf("autorização: %+v", a)
	}

	a, err = parse([]byte(`{"valid": false, "reason": "autorização expirada"}`))
	if err != nil || a.Valid || a.Reason != "autorização expirada" {
		t.Fatalf("recusa: %+v, %v", a, err)
	}
	if _, err := parse([]byte(`não é JSON`)); err == nil {
		t.Fatal("resposta inválida deveria falhar")
	}
	if ref := keyRef("accessLog:123"); ref["@assetType"] != "accessLog" || ref["@key"] != "accessLog:123" {
		t.Fatalf("referência: %v", ref)
	}
}

// Respostas 4xx do chaincode são recusas; falhas de comunicação e erros 5xx
// não são interpretados como resposta do livro-razão.
func TestChaincodeRefusal(t *testing.T) {
	st, err := status.New(codes.Aborted, "evaluate call to endorser returned error").WithDetails(&gateway.ErrorDetail{
		Address: "peer0.org1.example.com:7051",
		MspId:   "Org1MSP",
		Message: "chaincode response 404, registro de auditoria não encontrado",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reason, ok := chaincodeRefusal(st.Err()); !ok || reason != "registro de auditoria não encontrado" {
		t.Fatalf("recusa = %q, %v", reason, ok)
	}

	for _, e := range []error{
		status.Error(codes.Unavailable, "connection refused"),
		status.Error(codes.Aborted, "chaincode response 500, falha interna"),
	} {
		if _, ok := chaincodeRefusal(e); ok {
			t.Errorf("%v não deveria ser tratado como recusa", e)
		}
	}
}
