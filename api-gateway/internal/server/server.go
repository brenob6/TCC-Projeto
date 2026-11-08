// Package server expõe a API HTTP consumida pelo frontend de demonstração.
// Cada requisição informa, no cabeçalho X-Org, a instituição em nome da qual
// a API assina as transações (Seção 5.3.3 do TCC).
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"poc-key-sharing/kmsclient"

	"api-gateway/internal/config"
	"api-gateway/internal/directory"
	"api-gateway/internal/fabric"
)

// Ledger é o subconjunto do cliente Fabric usado pelos handlers.
type Ledger interface {
	Evaluate(org, fn string, args, transient any) (json.RawMessage, error)
	Submit(org, fn string, args, transient any) (*fabric.Result, error)
}

// Storage é o subconjunto do cliente IPFS usado pelos handlers.
type Storage interface {
	Add(ctx context.Context, name string, data []byte) (string, error)
	Cat(ctx context.Context, cid string) ([]byte, error)
	Unpin(ctx context.Context, cid string) error
	ID(ctx context.Context) (string, error)
}

// KeyService é o subconjunto do KMS usado pelos handlers.
type KeyService interface {
	GenerateAndDistribute(ctx context.Context, keyID string) ([]byte, error)
	Encrypt(pubKey, plaintext []byte) (*kmsclient.EncryptedRecord, error)
	Decrypt(ctx context.Context, keyID string, rec *kmsclient.EncryptedRecord, audit kmsclient.AuditInfo) ([]byte, error)
	HealthCheck(ctx context.Context) map[string]string
	Threshold() int
	Total() int
}

// Server reúne as dependências da API.
type Server struct {
	cfg    config.Config
	ledger Ledger
	ipfs   map[string]Storage // por organização
	kms    KeyService
	dir    *directory.Store
}

// New cria o servidor.
func New(cfg config.Config, ledger Ledger, storages map[string]Storage, kms KeyService, dir *directory.Store) *Server {
	return &Server{cfg: cfg, ledger: ledger, ipfs: storages, kms: kms, dir: dir}
}

// Handler registra as rotas.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/directory", s.directory)
	mux.HandleFunc("GET /api/assets/{key}", s.readAsset)

	mux.HandleFunc("POST /api/institutions", s.registerInstitution)
	mux.HandleFunc("PUT /api/institutions/status", s.setInstitutionStatus)

	mux.HandleFunc("POST /api/professionals", s.registerProfessional)
	mux.HandleFunc("GET /api/professionals/{key}", s.readAsset)
	mux.HandleFunc("PUT /api/professionals/{key}/status", s.setProfessionalStatus)

	mux.HandleFunc("POST /api/patients", s.registerPatient)
	mux.HandleFunc("POST /api/patients/lookup", s.lookupPatient)
	mux.HandleFunc("GET /api/patients/{key}", s.readAsset)
	mux.HandleFunc("GET /api/patients/{key}/records", s.listPatientRecords)
	mux.HandleFunc("GET /api/patients/{key}/all-records", s.allPatientRecords)
	mux.HandleFunc("GET /api/patients/{key}/permissions", s.listPermissions)
	mux.HandleFunc("POST /api/patients/{key}/permissions", s.grantPermission)
	mux.HandleFunc("GET /api/patients/{key}/check", s.checkPermission)
	mux.HandleFunc("GET /api/patients/{key}/history", s.accessHistory)
	mux.HandleFunc("GET /api/patients/{key}/emergencies", s.listEmergencies)

	mux.HandleFunc("POST /api/permissions/{key}/revoke", s.revokePermission)

	mux.HandleFunc("POST /api/records", s.addRecord)
	mux.HandleFunc("POST /api/records/{cid}/access", s.accessRecord)
	mux.HandleFunc("POST /api/records/{cid}/emergency", s.emergencyAccess)
	mux.HandleFunc("POST /api/records/{cid}/invalidate", s.invalidateRecord)

	if s.cfg.StaticDir != "" {
		mux.Handle("/", spa(s.cfg.StaticDir))
	}
	return logRequests(mux)
}

// ---- utilitários HTTP ----

type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string { return e.message }

func badRequest(format string, a ...any) error {
	return &httpError{status: http.StatusBadRequest, message: fmt.Sprintf(format, a...)}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("serializando resposta: %v", err)
	}
}

func writeError(w http.ResponseWriter, err error, trace *Trace) {
	status := http.StatusInternalServerError
	var he *httpError
	var fe *fabric.Error
	switch {
	case errors.As(err, &he):
		status = he.status
	case errors.As(err, &fe):
		status = fe.Status
	}
	body := map[string]any{"error": err.Error()}
	if trace != nil {
		body["trace"] = trace.Steps
	}
	writeJSON(w, status, body)
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return badRequest("corpo da requisição inválido: %v", err)
	}
	return nil
}

// org valida o cabeçalho X-Org.
func (s *Server) org(r *http.Request) (config.Org, error) {
	name := r.Header.Get("X-Org")
	org, ok := s.cfg.Org(name)
	if !ok {
		return config.Org{}, badRequest("informe a organização no cabeçalho X-Org (recebido %q)", name)
	}
	return org, nil
}

// institution devolve a instituição cadastrada para a organização.
func (s *Server) institution(org config.Org) (directory.Institution, error) {
	inst, ok := s.dir.InstitutionOf(org.Name)
	if !ok {
		return inst, badRequest("a organização %s ainda não cadastrou sua instituição", org.Name)
	}
	return inst, nil
}

// keyRef monta a referência a um ativo a partir da chave do cc-tools
// ("tipo:uuid").
func keyRef(key string) map[string]any {
	assetType, _, _ := strings.Cut(key, ":")
	return map[string]any{"@assetType": assetType, "@key": key}
}

func recordRef(cid string) map[string]any {
	return map[string]any{"@assetType": "medicalRecord", "cid": cid}
}

func (s *Server) readKey(org, key string) (map[string]any, error) {
	out, err := s.ledger.Evaluate(org, "readAsset", map[string]any{"key": keyRef(key)}, nil)
	if err != nil {
		return nil, err
	}
	var asset map[string]any
	return asset, json.Unmarshal(out, &asset)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s [%s] %s", r.Method, r.URL.Path, r.Header.Get("X-Org"), time.Since(start).Round(time.Millisecond))
		}
	})
}

// spa serve o build do frontend, devolvendo index.html para rotas
// desconhecidas.
func spa(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
}
