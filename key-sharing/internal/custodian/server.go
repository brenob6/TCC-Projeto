package custodian

import (
	"context"
	"errors"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"poc-key-sharing/internal/crypto"
	"poc-key-sharing/pb"
)

// Server implementa o CustodianService definido em proto/custodian.proto.
type Server struct {
	pb.UnimplementedCustodianServiceServer

	id    string
	store *Store
}

// NewServer cria o serviço gRPC de um custodiante identificado por id.
func NewServer(id string, store *Store) *Server {
	return &Server{id: id, store: store}
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
func (s *Server) PartialDecrypt(_ context.Context, req *pb.PartialDecryptRequest) (*pb.PartialDecryptResponse, error) {
	if req.GetKeyId() == "" {
		return nil, status.Error(codes.InvalidArgument, "key_id é obrigatório")
	}
	if len(req.GetC1()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "c1 é obrigatório")
	}

	audit := req.GetAudit()
	// No acesso emergencial a justificativa é obrigatória, pois é ela que
	// sustenta a auditoria da recuperação extraordinária.
	if audit.GetEmergency() && audit.GetReason() == "" {
		return nil, status.Error(codes.InvalidArgument, "acesso emergencial exige justificativa")
	}

	index, value, err := s.store.GetShare(req.GetKeyId())
	if errors.Is(err, ErrShareNotFound) {
		return nil, status.Errorf(codes.NotFound, "sem compartilhamento para a chave %s", req.GetKeyId())
	}
	if err != nil {
		log.Printf("[%s] erro ao ler share de %s: %v", s.id, req.GetKeyId(), err)
		return nil, status.Error(codes.Internal, "falha ao recuperar compartilhamento")
	}

	partial, err := crypto.PartialDecrypt(value, req.GetC1())
	if err != nil {
		log.Printf("[%s] erro na decifração parcial de %s: %v", s.id, req.GetKeyId(), err)
		return nil, status.Error(codes.InvalidArgument, "falha na decifração parcial")
	}

	if err := s.store.LogAccess(req.GetKeyId(), audit.GetRequesterId(),
		audit.GetInstitutionId(), audit.GetEmergency(), audit.GetReason()); err != nil {
		// A auditoria é requisito do sistema: sem registro, o acesso é negado.
		log.Printf("[%s] erro ao registrar auditoria de %s: %v", s.id, req.GetKeyId(), err)
		return nil, status.Error(codes.Internal, "falha ao registrar auditoria")
	}

	log.Printf("[%s] decifração parcial da chave %s para %s@%s (emergencial=%t)",
		s.id, req.GetKeyId(), audit.GetRequesterId(), audit.GetInstitutionId(), audit.GetEmergency())

	return &pb.PartialDecryptResponse{
		CustodianId: s.id,
		ShareIndex:  int32(index),
		Partial:     partial,
	}, nil
}

// Health informa o estado do custodiante.
func (s *Server) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	n, err := s.store.CountShares()
	if err != nil {
		return nil, status.Error(codes.Internal, "falha ao consultar base local")
	}
	return &pb.HealthResponse{CustodianId: s.id, ShareCount: int32(n)}, nil
}
