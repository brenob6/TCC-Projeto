package kms

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"

	"poc-key-sharing/internal/custodian"
	"poc-key-sharing/pb"
)

// startCustodians sobe n custodiantes reais em portas locais e devolve seus
// endereços, além de uma função para consultar a base de cada um.
func startCustodians(t *testing.T, n int) (addrs []string, stores []*custodian.Store) {
	t.Helper()

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("custodian-%d", i+1)

		store, err := custodian.OpenStore(filepath.Join(t.TempDir(), "shares.db"))
		if err != nil {
			t.Fatalf("abrindo base de %s: %v", id, err)
		}
		t.Cleanup(func() { store.Close() })

		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("escutando para %s: %v", id, err)
		}

		server := grpc.NewServer()
		pb.RegisterCustodianServiceServer(server, custodian.NewServer(id, store))
		go server.Serve(lis)
		t.Cleanup(server.Stop)

		addrs = append(addrs, lis.Addr().String())
		stores = append(stores, store)
	}
	return addrs, stores
}

// enderecoSemServidor reserva uma porta local e a libera em seguida, devolvendo
// um endereço onde nenhum custodiante atende.
func enderecoSemServidor(t *testing.T) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reservando porta: %v", err)
	}
	addr := lis.Addr().String()
	lis.Close()
	return addr
}

func newTestKMS(t *testing.T, threshold int, addrs []string) *KMS {
	t.Helper()

	service, err := New(Config{
		Threshold:  threshold,
		Custodians: addrs,
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("criando KMS: %v", err)
	}
	t.Cleanup(service.Close)
	return service
}

// Fluxo completo: geração da chave, distribuição dos shares, cifração e
// recuperação autorizada do prontuário.
func TestFluxoCompleto(t *testing.T) {
	addrs, _ := startCustodians(t, 3)
	service := newTestKMS(t, 2, addrs)
	ctx := context.Background()

	pubKey, err := service.GenerateAndDistribute(ctx, "paciente-001")
	if err != nil {
		t.Fatalf("gerando e distribuindo chave: %v", err)
	}

	prontuario := []byte("Paciente 001 - hipertensão arterial")
	rec, err := service.Encrypt(pubKey, prontuario)
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}

	got, err := service.Decrypt(ctx, "paciente-001", rec, AuditInfo{
		RequesterID:   "medico-001",
		InstitutionID: "hospital-a",
	})
	if err != nil {
		t.Fatalf("decifrando: %v", err)
	}
	if !bytes.Equal(got, prontuario) {
		t.Errorf("prontuário recuperado = %q, esperado %q", got, prontuario)
	}
}

// Com n=3 e k=2, a queda de um custodiante não impede a recuperação.
func TestToleraCustodianteIndisponivel(t *testing.T) {
	addrs, _ := startCustodians(t, 3)
	ctx := context.Background()

	service := newTestKMS(t, 2, addrs)
	pubKey, err := service.GenerateAndDistribute(ctx, "paciente-002")
	if err != nil {
		t.Fatalf("gerando e distribuindo chave: %v", err)
	}

	prontuario := []byte("Paciente 002 - diabetes tipo II")
	rec, err := service.Encrypt(pubKey, prontuario)
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}

	// O terceiro custodiante segue configurado, mas aponta para uma porta sem
	// servidor: com k=2, os dois restantes bastam para recuperar o prontuário.
	comQueda := []string{addrs[0], addrs[1], enderecoSemServidor(t)}
	parcial := newTestKMS(t, 2, comQueda)
	got, err := parcial.Decrypt(ctx, "paciente-002", rec, AuditInfo{
		RequesterID:   "medico-002",
		InstitutionID: "hospital-b",
	})
	if err != nil {
		t.Fatalf("decifrando com um custodiante fora: %v", err)
	}
	if !bytes.Equal(got, prontuario) {
		t.Errorf("prontuário recuperado = %q, esperado %q", got, prontuario)
	}
}

// Abaixo do limiar a recuperação deve ser recusada.
func TestLimiarNaoAtingido(t *testing.T) {
	addrs, _ := startCustodians(t, 3)
	ctx := context.Background()

	service := newTestKMS(t, 2, addrs)
	pubKey, err := service.GenerateAndDistribute(ctx, "paciente-003")
	if err != nil {
		t.Fatalf("gerando e distribuindo chave: %v", err)
	}

	rec, err := service.Encrypt(pubKey, []byte("Paciente 003 - alergia a penicilina"))
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}

	// Os três custodiantes seguem configurados, mas apenas um responde: os
	// outros dois apontam para portas sem servidor. Com uma única decifração
	// parcial o limiar k=2 não é alcançado.
	indisponiveis := []string{addrs[0], enderecoSemServidor(t), enderecoSemServidor(t)}
	insuficiente := newTestKMS(t, 2, indisponiveis)
	if _, err := insuficiente.Decrypt(ctx, "paciente-003", rec, AuditInfo{
		RequesterID:   "medico-003",
		InstitutionID: "hospital-c",
	}); err == nil {
		t.Error("recuperação abaixo do limiar deveria falhar")
	}
}

// A recuperação extraordinária é registrada como emergencial na auditoria de
// cada custodiante que participou da operação.
func TestRecuperacaoExtraordinariaRegistraAuditoria(t *testing.T) {
	addrs, stores := startCustodians(t, 3)
	service := newTestKMS(t, 2, addrs)
	ctx := context.Background()

	pubKey, err := service.GenerateAndDistribute(ctx, "paciente-004")
	if err != nil {
		t.Fatalf("gerando e distribuindo chave: %v", err)
	}

	prontuario := []byte("Paciente 004 - tipo sanguíneo O-")
	rec, err := service.Encrypt(pubKey, prontuario)
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}

	const motivo = "paciente inconsciente em atendimento de urgência"
	got, err := service.Decrypt(ctx, "paciente-004", rec, AuditInfo{
		RequesterID:   "medico-plantao",
		InstitutionID: "hospital-emergencia",
		Emergency:     true,
		Reason:        motivo,
	})
	if err != nil {
		t.Fatalf("recuperação extraordinária: %v", err)
	}
	if !bytes.Equal(got, prontuario) {
		t.Errorf("prontuário recuperado = %q, esperado %q", got, prontuario)
	}

	// Todos os custodiantes são consultados em paralelo, portanto cada um
	// deve ter registrado o acesso emergencial.
	for i, store := range stores {
		entries, err := store.AccessLog()
		if err != nil {
			t.Fatalf("lendo auditoria do custodiante %d: %v", i+1, err)
		}
		if len(entries) != 1 {
			t.Fatalf("custodiante %d: %d registros de auditoria, esperado 1", i+1, len(entries))
		}

		e := entries[0]
		if !e.Emergency {
			t.Errorf("custodiante %d: acesso não marcado como emergencial", i+1)
		}
		if e.Reason != motivo {
			t.Errorf("custodiante %d: justificativa = %q, esperado %q", i+1, e.Reason, motivo)
		}
		if e.RequesterID != "medico-plantao" || e.InstitutionID != "hospital-emergencia" {
			t.Errorf("custodiante %d: origem = %s@%s, esperado medico-plantao@hospital-emergencia",
				i+1, e.RequesterID, e.InstitutionID)
		}
	}
}

// O acesso emergencial sem justificativa deve ser recusado pelo KMS.
func TestEmergenciaSemJustificativaFalha(t *testing.T) {
	addrs, _ := startCustodians(t, 3)
	service := newTestKMS(t, 2, addrs)
	ctx := context.Background()

	pubKey, err := service.GenerateAndDistribute(ctx, "paciente-005")
	if err != nil {
		t.Fatalf("gerando e distribuindo chave: %v", err)
	}
	rec, err := service.Encrypt(pubKey, []byte("Paciente 005 - exames laboratoriais"))
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}

	if _, err := service.Decrypt(ctx, "paciente-005", rec, AuditInfo{
		RequesterID:   "medico-005",
		InstitutionID: "hospital-e",
		Emergency:     true,
	}); err == nil {
		t.Error("acesso emergencial sem justificativa deveria falhar")
	}
}

// Uma chave desconhecida não pode ser recuperada.
func TestChaveInexistenteFalha(t *testing.T) {
	addrs, _ := startCustodians(t, 3)
	service := newTestKMS(t, 2, addrs)
	ctx := context.Background()

	pubKey, err := service.GenerateAndDistribute(ctx, "paciente-006")
	if err != nil {
		t.Fatalf("gerando e distribuindo chave: %v", err)
	}
	rec, err := service.Encrypt(pubKey, []byte("Paciente 006 - anamnese"))
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}

	if _, err := service.Decrypt(ctx, "paciente-inexistente", rec, AuditInfo{
		RequesterID:   "medico-006",
		InstitutionID: "hospital-f",
	}); err == nil {
		t.Error("recuperação de chave inexistente deveria falhar")
	}
}

func TestNewRejeitaConfiguracaoInvalida(t *testing.T) {
	casos := []struct {
		nome string
		cfg  Config
	}{
		{"limiar menor que dois", Config{Threshold: 1, Custodians: []string{"a:1", "b:2"}}},
		{"custodiantes insuficientes", Config{Threshold: 3, Custodians: []string{"a:1", "b:2"}}},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			service, err := New(c.cfg)
			if err == nil {
				service.Close()
				t.Error("configuração inválida deveria ser recusada")
			}
		})
	}
}

// O KMS consulta o estado de cada custodiante configurado.
func TestHealthCheck(t *testing.T) {
	addrs, _ := startCustodians(t, 3)
	service := newTestKMS(t, 2, addrs)
	ctx := context.Background()

	if _, err := service.GenerateAndDistribute(ctx, "paciente-007"); err != nil {
		t.Fatalf("gerando e distribuindo chave: %v", err)
	}

	statuses := service.HealthCheck(ctx)
	if len(statuses) != len(addrs) {
		t.Fatalf("%d estados retornados, esperado %d", len(statuses), len(addrs))
	}
	for addr, status := range statuses {
		if !bytes.Contains([]byte(status), []byte("ativo")) {
			t.Errorf("custodiante %s: estado %q, esperado ativo", addr, status)
		}
	}
}
