package kms

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"poc-key-sharing/internal/custodian"
	"poc-key-sharing/pb"
)

// fakeLedger simula a transação getAccessAuthorization: devolve a autorização
// cadastrada para cada registro de auditoria e conta as consultas.
type fakeLedger struct {
	mu      sync.Mutex
	entries map[string]*custodian.Authorization
	err     error
	calls   int
}

func newFakeLedger() *fakeLedger {
	return &fakeLedger{entries: map[string]*custodian.Authorization{}}
}

func (f *fakeLedger) Authorize(_ context.Context, accessLog string) (*custodian.Authorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if a, ok := f.entries[accessLog]; ok {
		return a, nil
	}
	return &custodian.Authorization{Reason: "registro de auditoria inexistente"}, nil
}

// authorize cadastra uma leitura autorizada do envelope.
func (f *fakeLedger) authorize(accessLog, keyID string, envelope []byte) *custodian.Authorization {
	f.mu.Lock()
	defer f.mu.Unlock()
	sum := sha256.Sum256(envelope)
	a := &custodian.Authorization{
		Valid:       true,
		KMSKeyID:    keyID,
		ContentHash: hex.EncodeToString(sum[:]),
		Actor:       "professional:medico-livro-razao",
		Institution: "institution:hospital-livro-razao",
	}
	f.entries[accessLog] = a
	return a
}

// ledgerFixture sobe três custodiantes que confirmam as autorizações no
// livro-razão simulado e cifra um prontuário para a chave keyID.
type ledgerFixture struct {
	ledger   *fakeLedger
	stores   []*custodian.Store
	service  *KMS
	envelope []byte
	plain    []byte
	pubKey   []byte
}

func newLedgerFixture(t *testing.T, keyID string) *ledgerFixture {
	t.Helper()
	ledger := newFakeLedger()
	addrs, stores := startCustodiansWith(t, 3, ledger)
	service := newTestKMS(t, 2, addrs)

	pubKey, err := service.GenerateAndDistribute(context.Background(), keyID)
	if err != nil {
		t.Fatalf("gerando e distribuindo chave: %v", err)
	}
	plain := []byte("Prontuário de " + keyID)
	rec, err := service.Encrypt(pubKey, plain)
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}
	return &ledgerFixture{ledger: ledger, stores: stores, service: service, envelope: envelopeOf(t, rec), plain: plain, pubKey: pubKey}
}

// Leitura autorizada: os custodiantes confirmam o registro e gravam, na
// auditoria local, os dados do livro-razão, e não os enviados pelo
// coordenador.
func TestLivroRazaoAutorizaDecifracao(t *testing.T) {
	f := newLedgerFixture(t, "paciente-101")
	f.ledger.authorize("accessLog:1", "paciente-101", f.envelope)

	got, err := f.service.Decrypt(context.Background(), "paciente-101", f.envelope, AuditInfo{
		AccessLog:     "accessLog:1",
		RequesterID:   "informado-pelo-coordenador",
		InstitutionID: "informada-pelo-coordenador",
	})
	if err != nil {
		t.Fatalf("decifrando com autorização: %v", err)
	}
	if !bytes.Equal(got, f.plain) {
		t.Fatalf("prontuário recuperado = %q, esperado %q", got, f.plain)
	}

	for i, store := range f.stores {
		entries, err := store.AccessLog()
		if err != nil {
			t.Fatalf("lendo auditoria do custodiante %d: %v", i+1, err)
		}
		if len(entries) != 1 {
			t.Fatalf("custodiante %d: %d registros, esperado 1", i+1, len(entries))
		}
		e := entries[0]
		if e.AccessLog != "accessLog:1" || e.RequesterID != "professional:medico-livro-razao" ||
			e.InstitutionID != "institution:hospital-livro-razao" {
			t.Errorf("custodiante %d: auditoria com dados que não vieram do livro-razão: %+v", i+1, e)
		}
	}
}

// expectDenied confere que a decifração foi recusada pelos custodiantes e que
// nenhum deles calculou uma decifração parcial.
func (f *ledgerFixture) expectDenied(t *testing.T, keyID string, envelope []byte, audit AuditInfo) {
	t.Helper()
	before := f.logged(t)
	_, err := f.service.Decrypt(context.Background(), keyID, envelope, audit)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("erro = %v, esperado ErrDenied", err)
	}
	if after := f.logged(t); after != before {
		t.Fatalf("%d decifrações parciais registradas após a recusa, esperado nenhuma", after-before)
	}
}

func (f *ledgerFixture) logged(t *testing.T) int {
	t.Helper()
	n := 0
	for _, store := range f.stores {
		entries, err := store.AccessLog()
		if err != nil {
			t.Fatalf("lendo auditoria: %v", err)
		}
		n += len(entries)
	}
	return n
}

// Sem registro de auditoria, nenhuma decifração ocorre: é o caso de uma API
// comprometida que tenta decifrar sem passar pela transação de leitura.
func TestLivroRazaoSemRegistroRecusa(t *testing.T) {
	f := newLedgerFixture(t, "paciente-102")
	f.expectDenied(t, "paciente-102", f.envelope, AuditInfo{RequesterID: "medico", InstitutionID: "hospital"})
	if f.ledger.calls != 0 {
		t.Errorf("livro-razão consultado %d vezes sem registro informado", f.ledger.calls)
	}
}

// Registro inexistente, negado ou expirado: o livro-razão responde valid=false.
func TestLivroRazaoRegistroInvalidoRecusa(t *testing.T) {
	f := newLedgerFixture(t, "paciente-103")
	// O registro negado aponta para a mesma chave e o mesmo envelope, de modo
	// que apenas a indicação de validade do livro-razão impede a decifração.
	negado := f.ledger.authorize("accessLog:negado", "paciente-103", f.envelope)
	negado.Valid = false
	negado.Reason = "o registro não corresponde a uma leitura autorizada"

	f.expectDenied(t, "paciente-103", f.envelope, AuditInfo{AccessLog: "accessLog:negado"})
	f.expectDenied(t, "paciente-103", f.envelope, AuditInfo{AccessLog: "accessLog:inexistente"})
}

// Um registro válido de um paciente não autoriza a chave de outro.
func TestLivroRazaoRegistroDeOutraChaveRecusa(t *testing.T) {
	f := newLedgerFixture(t, "paciente-104")
	f.ledger.authorize("accessLog:outro", "paciente-outro", f.envelope)

	f.expectDenied(t, "paciente-104", f.envelope, AuditInfo{AccessLog: "accessLog:outro"})
}

// Um registro válido para um prontuário não autoriza decifrar outro
// prontuário do mesmo paciente, cifrado com a mesma chave.
func TestLivroRazaoEnvelopeDeOutroProntuarioRecusa(t *testing.T) {
	f := newLedgerFixture(t, "paciente-105")
	f.ledger.authorize("accessLog:a", "paciente-105", f.envelope)

	rec, err := f.service.Encrypt(f.pubKey, []byte("outro prontuário do mesmo paciente"))
	if err != nil {
		t.Fatalf("cifrando o segundo prontuário: %v", err)
	}
	f.expectDenied(t, "paciente-105", envelopeOf(t, rec), AuditInfo{AccessLog: "accessLog:a"})
}

// O mesmo registro de auditoria autoriza uma única decifração.
func TestLivroRazaoRegistroUsoUnico(t *testing.T) {
	f := newLedgerFixture(t, "paciente-106")
	f.ledger.authorize("accessLog:1", "paciente-106", f.envelope)
	audit := AuditInfo{AccessLog: "accessLog:1"}

	if _, err := f.service.Decrypt(context.Background(), "paciente-106", f.envelope, audit); err != nil {
		t.Fatalf("primeira decifração: %v", err)
	}
	f.expectDenied(t, "paciente-106", f.envelope, audit)
}

// Com o livro-razão inacessível, os custodiantes recusam a operação.
func TestLivroRazaoIndisponivelRecusa(t *testing.T) {
	f := newLedgerFixture(t, "paciente-107")
	f.ledger.authorize("accessLog:1", "paciente-107", f.envelope)
	f.ledger.err = errors.New("peer inacessível")

	if _, err := f.service.Decrypt(context.Background(), "paciente-107", f.envelope, AuditInfo{AccessLog: "accessLog:1"}); err == nil {
		t.Fatal("decifração sem confirmação no livro-razão deveria falhar")
	}
	if n := f.logged(t); n != 0 {
		t.Fatalf("%d decifrações parciais sem confirmação", n)
	}
}

// No acesso extraordinário, a justificativa registrada nos custodiantes é a do
// livro-razão.
func TestLivroRazaoEmergenciaRegistraJustificativaDoLivroRazao(t *testing.T) {
	f := newLedgerFixture(t, "paciente-108")
	a := f.ledger.authorize("accessLog:em", "paciente-108", f.envelope)
	a.Emergency = true
	a.EmergencyReason = "paciente inconsciente no pronto-socorro"

	if _, err := f.service.Decrypt(context.Background(), "paciente-108", f.envelope, AuditInfo{
		AccessLog: "accessLog:em",
		Emergency: true,
		Reason:    "justificativa informada pelo coordenador",
	}); err != nil {
		t.Fatalf("recuperação extraordinária: %v", err)
	}
	for i, store := range f.stores {
		entries, _ := store.AccessLog()
		if len(entries) != 1 || !entries[0].Emergency || entries[0].Reason != a.EmergencyReason {
			t.Errorf("custodiante %d: auditoria emergencial %+v", i+1, entries)
		}
	}
}

// Os custodiantes recusam conexões sem TLS e clientes cujo certificado não foi
// emitido pela CA do ambiente.
func TestCustodianteExigeTLSMutuo(t *testing.T) {
	f := newLedgerFixture(t, "paciente-109")
	addr := f.service.cfg.Custodians[0]

	semCertificado := testPKI(t).Client.Clone()
	semCertificado.Certificates = nil
	clientes := map[string]grpc.DialOption{
		"sem TLS":                    grpc.WithTransportCredentials(insecure.NewCredentials()),
		"certificado de outra CA":    grpc.WithTransportCredentials(credentials.NewTLS(testPKI(t).Intruder)),
		"sem certificado do cliente": grpc.WithTransportCredentials(credentials.NewTLS(semCertificado)),
	}
	for nome, opt := range clientes {
		t.Run(nome, func(t *testing.T) {
			conn, err := grpc.NewClient(addr, opt)
			if err != nil {
				t.Fatalf("criando cliente: %v", err)
			}
			defer conn.Close()
			if _, err := pb.NewCustodianServiceClient(conn).Health(context.Background(), &pb.HealthRequest{}); err == nil {
				t.Fatal("custodiante aceitou cliente não autenticado")
			}
		})
	}
}
