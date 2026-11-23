package custodian

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"poc-key-sharing/internal/crypto"
	"poc-key-sharing/pb"
)

// Authorization é a resposta do livro-razão à consulta de um registro de
// auditoria (transação getAccessAuthorization do chaincode).
type Authorization struct {
	Valid           bool
	Reason          string // motivo da recusa, quando Valid é falso
	KMSKeyID        string // chave do paciente a que o prontuário pertence
	ContentHash     string // SHA-256 do envelope registrado no livro-razão
	Actor           string // profissional que solicitou a leitura
	Institution     string // instituição do profissional
	Emergency       bool   // leitura por acesso extraordinário
	EmergencyReason string // justificativa registrada no acesso extraordinário
}

// Authorizer consulta o livro-razão para confirmar que um registro de
// auditoria autoriza a decifração.
type Authorizer interface {
	Authorize(ctx context.Context, accessLog string) (*Authorization, error)
}

// Server implementa o CustodianService definido em proto/custodian.proto.
type Server struct {
	pb.UnimplementedCustodianServiceServer

	id    string
	store *Store
	// auth é nulo apenas no modo isolado, usado nos testes do módulo sem a
	// rede blockchain; nesse modo, os dados de auditoria vêm do coordenador.
	auth Authorizer
}

// NewServer cria o serviço gRPC de um custodiante identificado por id. Com
// auth não nulo, toda decifração parcial exige a confirmação da autorização
// no livro-razão.
func NewServer(id string, store *Store, auth Authorizer) *Server {
	return &Server{id: id, store: store, auth: auth}
}

// StoreShare persiste o compartilhamento enviado pelo KMS.
func (s *Server) StoreShare(_ context.Context, req *pb.StoreShareRequest) (*pb.StoreShareResponse, error) {
	if req.GetKeyId() == "" {
		return nil, status.Error(codes.InvalidArgument, "key_id é obrigatório")
	}
	if len(req.GetShareValue()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "share_value é obrigatório")
	}

	if err := s.store.PutShare(req.GetKeyId(), int(req.GetShareIndex()), req.GetShareValue()); err != nil {
		log.Printf("[%s] erro ao armazenar share de %s: %v", s.id, req.GetKeyId(), err)
		return nil, status.Error(codes.Internal, "falha ao armazenar compartilhamento")
	}

	log.Printf("[%s] share %d armazenado para a chave %s", s.id, req.GetShareIndex(), req.GetKeyId())
	return &pb.StoreShareResponse{CustodianId: s.id, Stored: true}, nil
}

// PartialDecrypt devolve D_i = C1^{x_i}. O share permanece no custodiante.
//
// Com a verificação no livro-razão habilitada, a operação só ocorre se o
// registro de auditoria informado corresponder a uma leitura autorizada e
// recente, da chave solicitada e do envelope recebido, e se esse registro
// ainda não tiver sido usado neste custodiante.
func (s *Server) PartialDecrypt(ctx context.Context, req *pb.PartialDecryptRequest) (*pb.PartialDecryptResponse, error) {
	keyID := req.GetKeyId()
	if keyID == "" {
		return nil, status.Error(codes.InvalidArgument, "key_id é obrigatório")
	}
	if len(req.GetEnvelope()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "envelope é obrigatório")
	}
	rec, err := crypto.ParseEnvelope(req.GetEnvelope())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	entry, err := s.authorize(ctx, req)
	if err != nil {
		return nil, err
	}

	index, value, err := s.store.GetShare(keyID)
	if errors.Is(err, ErrShareNotFound) {
		return nil, status.Errorf(codes.NotFound, "sem compartilhamento para a chave %s", keyID)
	}
	if err != nil {
		log.Printf("[%s] erro ao ler share de %s: %v", s.id, keyID, err)
		return nil, status.Error(codes.Internal, "falha ao recuperar compartilhamento")
	}

	// O registro local precede o cálculo: sem auditoria, o acesso é negado, e
	// a gravação consome a autorização do livro-razão.
	switch err := s.store.LogAccess(entry); {
	case errors.Is(err, ErrAuthorizationUsed):
		log.Printf("[%s] autorização %s reapresentada para a chave %s", s.id, entry.AccessLog, keyID)
		return nil, status.Error(codes.PermissionDenied, "o registro de auditoria já autorizou uma decifração neste custodiante")
	case err != nil:
		log.Printf("[%s] erro ao registrar auditoria de %s: %v", s.id, keyID, err)
		return nil, status.Error(codes.Internal, "falha ao registrar auditoria")
	}

	partial, err := crypto.PartialDecrypt(value, rec.C1)
	if err != nil {
		log.Printf("[%s] erro na decifração parcial de %s: %v", s.id, keyID, err)
		return nil, status.Error(codes.InvalidArgument, "falha na decifração parcial")
	}

	log.Printf("[%s] decifração parcial da chave %s para %s@%s (emergencial=%t, autorização=%s)",
		s.id, keyID, entry.RequesterID, entry.InstitutionID, entry.Emergency, orDash(entry.AccessLog))

	return &pb.PartialDecryptResponse{
		CustodianId: s.id,
		ShareIndex:  int32(index),
		Partial:     partial,
	}, nil
}

// authorize decide se a decifração parcial pode ocorrer e devolve a entrada
// de auditoria correspondente.
func (s *Server) authorize(ctx context.Context, req *pb.PartialDecryptRequest) (AccessEntry, error) {
	keyID := req.GetKeyId()
	audit := req.GetAudit()

	if s.auth == nil {
		// Modo isolado: no acesso emergencial a justificativa é obrigatória,
		// pois é ela que sustenta a auditoria da recuperação extraordinária.
		if audit.GetEmergency() && audit.GetReason() == "" {
			return AccessEntry{}, status.Error(codes.InvalidArgument, "acesso emergencial exige justificativa")
		}
		return AccessEntry{
			KeyID:         keyID,
			RequesterID:   audit.GetRequesterId(),
			InstitutionID: audit.GetInstitutionId(),
			Emergency:     audit.GetEmergency(),
			Reason:        audit.GetReason(),
		}, nil
	}

	accessLog := req.GetAccessLog()
	if accessLog == "" {
		return AccessEntry{}, denied("o registro de auditoria do livro-razão é obrigatório")
	}
	auth, err := s.auth.Authorize(ctx, accessLog)
	if err != nil {
		// Sem confirmação, a decifração é recusada.
		log.Printf("[%s] falha ao consultar o livro-razão sobre %s: %v", s.id, accessLog, err)
		return AccessEntry{}, status.Error(codes.Unavailable, "não foi possível confirmar a autorização no livro-razão")
	}
	if !auth.Valid {
		log.Printf("[%s] autorização %s recusada pelo livro-razão: %s", s.id, accessLog, auth.Reason)
		return AccessEntry{}, denied("autorização recusada pelo livro-razão: " + auth.Reason)
	}
	if auth.KMSKeyID != keyID {
		log.Printf("[%s] autorização %s refere-se à chave %s, e não a %s", s.id, accessLog, auth.KMSKeyID, keyID)
		return AccessEntry{}, denied("o registro de auditoria não se refere à chave solicitada")
	}
	sum := sha256.Sum256(req.GetEnvelope())
	if hex.EncodeToString(sum[:]) != auth.ContentHash {
		log.Printf("[%s] envelope apresentado com %s difere do prontuário autorizado", s.id, accessLog)
		return AccessEntry{}, denied("o envelope não corresponde ao prontuário autorizado")
	}

	// Os dados de auditoria vêm do livro-razão; os informados pelo
	// coordenador são desconsiderados.
	return AccessEntry{
		KeyID:         keyID,
		RequesterID:   auth.Actor,
		InstitutionID: auth.Institution,
		Emergency:     auth.Emergency,
		Reason:        auth.EmergencyReason,
		AccessLog:     accessLog,
	}, nil
}

func denied(msg string) error {
	return status.Error(codes.PermissionDenied, msg)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Health informa o estado do custodiante.
func (s *Server) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	n, err := s.store.CountShares()
	if err != nil {
		return nil, status.Error(codes.Internal, "falha ao consultar base local")
	}
	return &pb.HealthResponse{
		CustodianId:        s.id,
		ShareCount:         int32(n),
		LedgerVerification: s.auth != nil,
	}, nil
}
