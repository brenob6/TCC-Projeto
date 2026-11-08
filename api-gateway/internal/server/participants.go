package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"api-gateway/internal/directory"
	"api-gateway/internal/fabric"
)

// health verifica o peer, o nó IPFS de cada organização e os custodiantes.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	type orgHealth struct {
		Name   string `json:"name"`
		MSPID  string `json:"mspId"`
		Fabric string `json:"fabric"`
		IPFS   string `json:"ipfs"`
	}
	// As verificações correm em paralelo e com prazo: com as máquinas
	// desligadas, cada tentativa só falharia após o timeout do cliente.
	kmsCtx, kmsCancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer kmsCancel()
	kmsDone := make(chan map[string]string, 1)
	go func() { kmsDone <- s.kms.HealthCheck(kmsCtx) }()

	var mu sync.Mutex
	orgs := make([]orgHealth, len(s.cfg.Orgs))
	done := make(chan struct{}, 2*len(s.cfg.Orgs))
	for i, org := range s.cfg.Orgs {
		orgs[i] = orgHealth{Name: org.Name, MSPID: org.MSPID, Fabric: "sem resposta", IPFS: "sem resposta"}
		go func() {
			defer func() { done <- struct{}{} }()
			out, err := s.ledger.Evaluate(org.Name, "getHeader", nil, nil)
			var header struct{ Name, Version string }
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				orgs[i].Fabric = err.Error()
			case json.Unmarshal(out, &header) == nil:
				orgs[i].Fabric = "ok (" + header.Name + " " + header.Version + ")"
			}
		}()
		go func() {
			defer func() { done <- struct{}{} }()
			id, err := s.ipfs[org.Name].ID(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				orgs[i].IPFS = err.Error()
			} else {
				orgs[i].IPFS = "ok (" + id + ")"
			}
		}()
	}
	for range 2 * len(s.cfg.Orgs) {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	mu.Lock()
	snapshot := append([]orgHealth{}, orgs...)
	mu.Unlock()

	custodians := <-kmsDone
	addrs := make([]string, 0, len(custodians))
	for a := range custodians {
		addrs = append(addrs, a)
	}
	sort.Strings(addrs)
	kms := []map[string]string{}
	for _, a := range addrs {
		kms = append(kms, map[string]string{"address": a, "status": custodians[a]})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"orgs":       snapshot,
		"custodians": kms,
		"threshold":  s.kms.Threshold(),
		"total":      s.kms.Total(),
	})
}

// directory devolve as organizações e os participantes conhecidos pela API.
func (s *Server) directory(w http.ResponseWriter, r *http.Request) {
	data := s.dir.Snapshot()
	orgs := []map[string]any{}
	for _, org := range s.cfg.Orgs {
		entry := map[string]any{"name": org.Name, "mspId": org.MSPID}
		if inst, ok := s.dir.InstitutionOf(org.Name); ok {
			entry["institution"] = inst
		}
		orgs = append(orgs, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orgs":          orgs,
		"professionals": data.Professionals,
		"patients":      data.Patients,
	})
}

// readAsset devolve um ativo pela chave informada na rota.
func (s *Server) readAsset(w http.ResponseWriter, r *http.Request) {
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	asset, err := s.readKey(org.Name, r.PathValue("key"))
	if err != nil {
		writeError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, asset)
}

func (s *Server) registerInstitution(w http.ResponseWriter, r *http.Request) {
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		CNPJ string `json:"cnpj"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}

	var asset map[string]any
	res, err := s.ledger.Submit(org.Name, "registerInstitution", req, nil)
	var fe *fabric.Error
	switch {
	case err == nil:
		if err := json.Unmarshal(res.Payload, &asset); err != nil {
			writeError(w, err, nil)
			return
		}
	case asFabric(err, &fe) && fe.Status == http.StatusConflict:
		// A organização já possui instituição no ledger (por exemplo, criada
		// pelo test-e2e.sh): sincroniza o diretório se o CNES corresponder.
		out, rerr := s.ledger.Evaluate(org.Name, "readAsset",
			map[string]any{"key": map[string]any{"@assetType": "institution", "id": req.ID}}, nil)
		if rerr != nil || json.Unmarshal(out, &asset) != nil || asset["mspId"] != org.MSPID {
			writeError(w, err, nil)
			return
		}
	default:
		writeError(w, err, nil)
		return
	}

	inst := directory.Institution{
		Org:  org.Name,
		Key:  str(asset["@key"]),
		ID:   str(asset["id"]),
		Name: str(asset["name"]),
	}
	if err := s.dir.PutInstitution(inst); err != nil {
		writeError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"institution": asset, "txId": txID(res)})
}

func (s *Server) setInstitutionStatus(w http.ResponseWriter, r *http.Request) {
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	inst, err := s.institution(org)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		Active bool `json:"active"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	res, err := s.ledger.Submit(org.Name, "setInstitutionStatus",
		map[string]any{"institution": keyRef(inst.Key), "active": req.Active}, nil)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"institution": res.Payload, "txId": res.TxID})
}

func (s *Server) registerProfessional(w http.ResponseWriter, r *http.Request) {
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	inst, err := s.institution(org)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		CPF                    string `json:"cpf"`
		Name                   string `json:"name"`
		ProfessionalType       string `json:"professionalType"`
		CouncilRegistry        string `json:"councilRegistry"`
		EmergencyAccessAllowed *bool  `json:"emergencyAccessAllowed"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	args := map[string]any{
		"name":             req.Name,
		"professionalType": req.ProfessionalType,
		"councilRegistry":  req.CouncilRegistry,
		"institution":      keyRef(inst.Key),
	}
	if req.EmergencyAccessAllowed != nil {
		args["emergencyAccessAllowed"] = *req.EmergencyAccessAllowed
	}
	res, err := s.ledger.Submit(org.Name, "registerProfessional", args, map[string]any{"cpf": onlyDigits(req.CPF)})
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var asset map[string]any
	if err := json.Unmarshal(res.Payload, &asset); err != nil {
		writeError(w, err, nil)
		return
	}
	err = s.dir.AddProfessional(directory.Professional{
		Org:              org.Name,
		Key:              str(asset["@key"]),
		ID:               str(asset["id"]),
		Name:             req.Name,
		ProfessionalType: req.ProfessionalType,
		CouncilRegistry:  req.CouncilRegistry,
	})
	if err != nil {
		writeError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"professional": asset, "txId": res.TxID})
}

func (s *Server) setProfessionalStatus(w http.ResponseWriter, r *http.Request) {
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		Active                 bool  `json:"active"`
		EmergencyAccessAllowed *bool `json:"emergencyAccessAllowed"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	args := map[string]any{"professional": keyRef(r.PathValue("key")), "active": req.Active}
	if req.EmergencyAccessAllowed != nil {
		args["emergencyAccessAllowed"] = *req.EmergencyAccessAllowed
	}
	res, err := s.ledger.Submit(org.Name, "setProfessionalStatus", args, nil)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"professional": res.Payload, "txId": res.TxID})
}

// registerPatient gera a chave de cifração no KMS (shares distribuídos aos
// custodiantes) e cadastra o paciente com a chave pública de assinatura
// gerada na carteira do próprio paciente.
func (s *Server) registerPatient(w http.ResponseWriter, r *http.Request) {
	trace := &Trace{}
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	if _, err := s.institution(org); err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		CPF              string `json:"cpf"`
		Name             string `json:"name"`
		SigningPublicKey string `json:"signingPublicKey"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	cpf := onlyDigits(req.CPF)
	if req.Name == "" || req.SigningPublicKey == "" {
		writeError(w, badRequest("nome e chave pública de assinatura são obrigatórios"), nil)
		return
	}

	// Evita gerar chave no KMS para um paciente já cadastrado ou com a rede
	// indisponível.
	var fe *fabric.Error
	_, err = s.ledger.Evaluate(org.Name, "getPatientByCpf", nil, map[string]any{"cpf": cpf})
	switch {
	case err == nil:
		writeError(w, &httpError{status: http.StatusConflict, message: "paciente já cadastrado"}, nil)
		return
	case !asFabric(err, &fe) || fe.Status != http.StatusNotFound:
		writeError(w, err, nil)
		return
	}
	trace.add("Blockchain", "getPatientByCpf", "CPF enviado no mapa transiente; paciente ainda não cadastrado")

	kmsKeyID := "paciente-" + uuid.NewString()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	pub, err := s.kms.GenerateAndDistribute(ctx, kmsKeyID)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	trace.add("KMS", "keygen", "chave %s gerada; %d shares distribuídos (limiar %d-de-%d)",
		kmsKeyID, s.kms.Total(), s.kms.Threshold(), s.kms.Total())

	res, err := s.ledger.Submit(org.Name, "registerPatient", map[string]any{
		"kmsKeyId":         kmsKeyID,
		"publicKey":        base64.StdEncoding.EncodeToString(pub),
		"signingPublicKey": req.SigningPublicKey,
	}, map[string]any{"cpf": cpf})
	if err != nil {
		writeError(w, err, trace)
		return
	}
	var asset map[string]any
	if err := json.Unmarshal(res.Payload, &asset); err != nil {
		writeError(w, err, trace)
		return
	}
	trace.tx("registerPatient", res.TxID, "ledger guarda apenas o UUID, o HMAC do CPF e as chaves públicas")

	err = s.dir.AddPatient(directory.Patient{
		Key:      str(asset["@key"]),
		ID:       str(asset["id"]),
		Name:     req.Name,
		Org:      org.Name,
		KMSKeyID: kmsKeyID,
	})
	if err != nil {
		writeError(w, err, trace)
		return
	}
	trace.add("API", "diretório", "nome do paciente mantido fora da blockchain")
	writeJSON(w, http.StatusOK, map[string]any{"patient": asset, "txId": res.TxID, "trace": trace.Steps})
}

func (s *Server) lookupPatient(w http.ResponseWriter, r *http.Request) {
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		CPF string `json:"cpf"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	out, err := s.ledger.Evaluate(org.Name, "getPatientByCpf", nil, map[string]any{"cpf": onlyDigits(req.CPF)})
	if err != nil {
		writeError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, json.RawMessage(out))
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func txID(res *fabric.Result) string {
	if res == nil {
		return ""
	}
	return res.TxID
}

func onlyDigits(s string) string {
	out := make([]rune, 0, len(s))
	for _, c := range s {
		if c >= '0' && c <= '9' {
			out = append(out, c)
		}
	}
	return string(out)
}
