// Package kms implementa o serviço de gerenciamento de chaves. O KMS gera as
// chaves dos pacientes, distribui os compartilhamentos entre os custodiantes e
// coordena a recuperação quando o acesso é autorizado.
package kms

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"poc-key-sharing/internal/crypto"
	"poc-key-sharing/pb"
)

// Config define o esquema de limiar e os custodiantes disponíveis.
type Config struct {
	Threshold  int      // k: mínimo de compartilhamentos para decifrar
	Custodians []string // endereços gRPC dos custodiantes (n = len)
	Timeout    time.Duration
}

// KMS coordena os custodiantes de chaves.
type KMS struct {
	cfg   Config
	conns []*grpc.ClientConn
	// clients[i] corresponde a cfg.Custodians[i].
	clients []pb.CustodianServiceClient
}

// New abre as conexões gRPC com todos os custodiantes configurados.
func New(cfg Config) (*KMS, error) {
	if cfg.Threshold < 2 {
		return nil, fmt.Errorf("limiar deve ser ao menos 2, recebido %d", cfg.Threshold)
	}
	if len(cfg.Custodians) < cfg.Threshold {
		return nil, fmt.Errorf("são necessários ao menos %d custodiantes, configurados %d",
			cfg.Threshold, len(cfg.Custodians))
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	k := &KMS{cfg: cfg}
	for _, addr := range cfg.Custodians {
		// A comunicação é interna à arquitetura; nesta prova de conceito o
		// canal é aberto sem TLS para simplificar a execução dos testes.
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			k.Close()
			return nil, fmt.Errorf("conectando ao custodiante %s: %w", addr, err)
		}
		k.conns = append(k.conns, conn)
		k.clients = append(k.clients, pb.NewCustodianServiceClient(conn))
	}
	return k, nil
}

// Close encerra as conexões com os custodiantes.
func (k *KMS) Close() {
	for _, c := range k.conns {
		c.Close()
	}
	k.conns, k.clients = nil, nil
}

// Total informa o número de custodiantes (n do esquema).
func (k *KMS) Total() int { return len(k.cfg.Custodians) }

// Threshold informa o limiar do esquema (k).
func (k *KMS) Threshold() int { return k.cfg.Threshold }

// GenerateAndDistribute gera a chave do paciente e envia um compartilhamento a
// cada custodiante. Retorna apenas a chave pública: os compartilhamentos não
// permanecem no KMS.
func (k *KMS) GenerateAndDistribute(ctx context.Context, keyID string) ([]byte, error) {
	total := k.Total()
	pubKey, shares, err := crypto.GenerateKey(k.cfg.Threshold, total)
	if err != nil {
		return nil, err
	}

	for i, sh := range shares {
		reqCtx, cancel := context.WithTimeout(ctx, k.cfg.Timeout)
		resp, err := k.clients[i].StoreShare(reqCtx, &pb.StoreShareRequest{
			KeyId:      keyID,
			ShareIndex: int32(sh.Index),
			ShareValue: sh.Value,
		})
		cancel()
		if err != nil {
			return nil, fmt.Errorf("distribuindo share %d para %s: %w",
				sh.Index, k.cfg.Custodians[i], err)
		}
		log.Printf("share %d entregue ao custodiante %s", sh.Index, resp.GetCustodianId())
	}

	log.Printf("chave %s gerada no esquema %d-de-%d", keyID, k.cfg.Threshold, total)
	return pubKey, nil
}

// Encrypt cifra um prontuário com a chave pública do paciente.
func (k *KMS) Encrypt(pubKey, plaintext []byte) (*crypto.EncryptedRecord, error) {
	return crypto.Encrypt(pubKey, plaintext)
}

// AuditInfo identifica a origem de uma solicitação de decifração.
type AuditInfo struct {
	RequesterID   string
	InstitutionID string
	Emergency     bool
	Reason        string
}

// Decrypt reúne decifrações parciais de pelo menos k custodiantes e recupera o
// prontuário. A chave privada não é reconstituída em nenhuma entidade.
//
// As solicitações são feitas em paralelo a todos os custodiantes; a decifração
// é bem-sucedida desde que ao menos k respondam, tolerando a indisponibilidade
// dos demais.
func (k *KMS) Decrypt(ctx context.Context, keyID string, rec *crypto.EncryptedRecord, audit AuditInfo) ([]byte, error) {
	if audit.Emergency && audit.Reason == "" {
		return nil, fmt.Errorf("acesso emergencial exige justificativa")
	}

	auditPB := &pb.AuditContext{
		RequesterId:   audit.RequesterID,
		InstitutionId: audit.InstitutionID,
		Emergency:     audit.Emergency,
		Reason:        audit.Reason,
	}

	type result struct {
		addr    string
		partial crypto.Partial
		err     error
	}

	results := make([]result, len(k.clients))
	var wg sync.WaitGroup
	for i, client := range k.clients {
		wg.Add(1)
		go func(i int, client pb.CustodianServiceClient) {
			defer wg.Done()
			reqCtx, cancel := context.WithTimeout(ctx, k.cfg.Timeout)
			defer cancel()

			resp, err := client.PartialDecrypt(reqCtx, &pb.PartialDecryptRequest{
				KeyId: keyID,
				C1:    rec.C1,
				Audit: auditPB,
			})
			results[i] = result{addr: k.cfg.Custodians[i], err: err}
			if err == nil {
				results[i].partial = crypto.Partial{
					Index: int(resp.GetShareIndex()),
					Value: resp.GetPartial(),
				}
			}
		}(i, client)
	}
	wg.Wait()

	var partials []crypto.Partial
	for _, r := range results {
		if r.err != nil {
			log.Printf("custodiante %s indisponível: %v", r.addr, r.err)
			continue
		}
		partials = append(partials, r.partial)
		log.Printf("decifração parcial recebida do custodiante %s (share %d)", r.addr, r.partial.Index)
	}

	if len(partials) < k.cfg.Threshold {
		return nil, fmt.Errorf("limiar não atingido: %d de %d decifrações parciais obtidas",
			len(partials), k.cfg.Threshold)
	}

	// Apenas k parciais são combinadas, evidenciando que o limiar é suficiente.
	return crypto.CombineAndDecrypt(rec, partials[:k.cfg.Threshold], k.cfg.Threshold, k.Total())
}

// HealthCheck consulta o estado de cada custodiante configurado.
func (k *KMS) HealthCheck(ctx context.Context) map[string]string {
	statuses := make(map[string]string, len(k.clients))
	for i, client := range k.clients {
		reqCtx, cancel := context.WithTimeout(ctx, k.cfg.Timeout)
		resp, err := client.Health(reqCtx, &pb.HealthRequest{})
		cancel()

		addr := k.cfg.Custodians[i]
		if err != nil {
			statuses[addr] = fmt.Sprintf("indisponível: %v", err)
			continue
		}
		statuses[addr] = fmt.Sprintf("ativo (%s, %d shares)", resp.GetCustodianId(), resp.GetShareCount())
	}
	return statuses
}
