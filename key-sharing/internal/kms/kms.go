// Package kms implementa o serviço de gerenciamento de chaves. O KMS gera as
// chaves dos pacientes, distribui os compartilhamentos entre os custodiantes e
// coordena a recuperação quando o acesso é autorizado.
package kms

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"poc-key-sharing/internal/crypto"
	"poc-key-sharing/pb"
)

// Config define o esquema de limiar e os custodiantes disponíveis.
type Config struct {
	Threshold  int      // k: mínimo de compartilhamentos para decifrar
	Custodians []string // endereços gRPC dos custodiantes (n = len)
	Timeout    time.Duration
	// TLS é a configuração do canal com os custodiantes, com o certificado
	// do coordenador para a autenticação mútua (ver tlsconf.Client).
	TLS *tls.Config
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
	if cfg.TLS == nil {
		return nil, fmt.Errorf("a configuração TLS dos custodiantes é obrigatória")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	k := &KMS{cfg: cfg}
	for _, addr := range cfg.Custodians {
		// TLS mútuo: o coordenador verifica o certificado do custodiante, e o
		// custodiante só aceita o coordenador cujo certificado foi emitido pela
		// CA do ambiente. Sem isso, as decifrações parciais trafegariam em
		// claro e poderiam ser combinadas por quem observasse a rede.
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(cfg.TLS.Clone())))
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
	// AccessLog é a chave do registro de auditoria gravado no livro-razão pela
	// transação de leitura. Os custodiantes o consultam antes de decifrar e
	// registram os dados obtidos no livro-razão, e não os campos abaixo, que
	// são usados apenas no modo isolado.
	AccessLog     string
	RequesterID   string
	InstitutionID string
	Emergency     bool
	Reason        string
}

// ErrDenied indica que os custodiantes recusaram a decifração por falta de
// autorização válida no livro-razão.
var ErrDenied = fmt.Errorf("decifração recusada pelos custodiantes")

// Decrypt reúne decifrações parciais de pelo menos k custodiantes e recupera o
// prontuário. A chave privada não é reconstituída em nenhuma entidade.
//
// envelope é o prontuário cifrado exatamente como armazenado no IPFS: cada
// custodiante confere seu resumo com o registrado no livro-razão.
//
// As solicitações são feitas em paralelo a todos os custodiantes; a decifração
// é bem-sucedida desde que ao menos k respondam, tolerando a indisponibilidade
// dos demais.
func (k *KMS) Decrypt(ctx context.Context, keyID string, envelope []byte, audit AuditInfo) ([]byte, error) {
	if audit.Emergency && audit.AccessLog == "" && audit.Reason == "" {
		return nil, fmt.Errorf("acesso emergencial exige justificativa")
	}
	rec, err := crypto.ParseEnvelope(envelope)
	if err != nil {
		return nil, err
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
				KeyId:     keyID,
				Audit:     auditPB,
				AccessLog: audit.AccessLog,
				Envelope:  envelope,
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
	var refusal error
	for _, r := range results {
		if r.err != nil {
			if status.Code(r.err) == codes.PermissionDenied {
				log.Printf("custodiante %s recusou a decifração: %v", r.addr, r.err)
				if refusal == nil {
					refusal = fmt.Errorf("%w: %s", ErrDenied, status.Convert(r.err).Message())
				}
				continue
			}
			log.Printf("custodiante %s indisponível: %v", r.addr, r.err)
			continue
		}
		partials = append(partials, r.partial)
		log.Printf("decifração parcial recebida do custodiante %s (share %d)", r.addr, r.partial.Index)
	}

	if len(partials) < k.cfg.Threshold {
		if refusal != nil {
			return nil, refusal
		}
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
		mode := "verificação no livro-razão"
		if !resp.GetLedgerVerification() {
			mode = "modo isolado, sem verificação no livro-razão"
		}
		statuses[addr] = fmt.Sprintf("ativo (%s, %d shares, %s)", resp.GetCustodianId(), resp.GetShareCount(), mode)
	}
	return statuses
}
