package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"poc-key-sharing/kmsclient"

	"api-gateway/internal/config"
	"api-gateway/internal/directory"
	"api-gateway/internal/fabric"
)

// document é o conteúdo do prontuário antes da cifração. Nada dele chega ao
// ledger.
type document struct {
	Title      string `json:"title"`
	RecordType string `json:"recordType"`
	Content    string `json:"content"`
	File       *struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Data string `json:"data"` // Base64
	} `json:"file,omitempty"`
}

// addRecord implementa a Figura 7: cifra o prontuário com a chave pública do
// paciente, armazena o envelope no IPFS e registra o CID na blockchain.
func (s *Server) addRecord(w http.ResponseWriter, r *http.Request) {
	trace := &Trace{}
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		document
		Patient string `json:"patient"`
		Author  string `json:"author"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	if req.Patient == "" || req.Author == "" || req.Title == "" {
		writeError(w, badRequest("paciente, autor e título são obrigatórios"), nil)
		return
	}

	// Verificação antecipada, sem auditoria, para não gravar no IPFS um
	// prontuário que o chaincode recusaria.
	out, err := s.ledger.Evaluate(org.Name, "checkPermission",
		map[string]any{"patient": keyRef(req.Patient), "professional": keyRef(req.Author)}, nil)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	var perm struct{ Write bool }
	if err := json.Unmarshal(out, &perm); err != nil {
		writeError(w, err, trace)
		return
	}
	if !perm.Write {
		trace.add("Blockchain", "checkPermission", "sem permissão de leitura e escrita")
		writeError(w, &httpError{status: http.StatusForbidden, message: "o paciente não concedeu permissão de escrita ao autor"}, trace)
		return
	}
	trace.add("Blockchain", "checkPermission", "permissão de leitura e escrita ativa")

	patient, err := s.readKey(org.Name, req.Patient)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	pub, err := base64.StdEncoding.DecodeString(str(patient["publicKey"]))
	if err != nil {
		writeError(w, fmt.Errorf("chave pública do paciente inválida: %w", err), trace)
		return
	}
	trace.add("KMS", "getPublicKey", "chave pública de %s (gerada pelo KMS no cadastro)", str(patient["kmsKeyId"]))

	plaintext, err := json.Marshal(req.document)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	rec, err := s.kms.Encrypt(pub, plaintext)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	envelope, err := json.Marshal(rec)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	sum := sha256.Sum256(envelope)
	contentHash := hex.EncodeToString(sum[:])
	trace.add("API", "encryptDocument", "ElGamal + AES-256-GCM: %d bytes em claro, envelope de %d bytes, SHA-256 %s…",
		len(plaintext), len(envelope), contentHash[:16])

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cid, err := s.ipfs[org.Name].Add(ctx, "prontuario.enc.json", envelope)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	trace.add("IPFS", "addDocument", "envelope cifrado armazenado no nó de %s: %s", org.Name, cid)

	res, err := s.ledger.Submit(org.Name, "registerMedicalRecord", map[string]any{
		"cid":         cid,
		"contentHash": contentHash,
		"patient":     keyRef(req.Patient),
		"author":      keyRef(req.Author),
		"recordType":  req.RecordType,
	}, nil)
	if err != nil {
		if uerr := s.ipfs[org.Name].Unpin(ctx, cid); uerr == nil {
			trace.add("IPFS", "unpin", "envelope removido após a recusa do chaincode")
		}
		writeError(w, err, trace)
		return
	}
	trace.tx("registerCID", res.TxID, "CID, hash, paciente, autor e tipo registrados; auditoria CRIACAO")

	var result struct {
		Record map[string]any `json:"record"`
	}
	_ = json.Unmarshal(res.Payload, &result)
	err = s.dir.AddRecord(directory.Record{
		CID:        cid,
		Key:        str(result.Record["@key"]),
		Patient:    req.Patient,
		Author:     req.Author,
		Org:        org.Name,
		RecordType: req.RecordType,
		Title:      req.Title,
		CreatedAt:  str(result.Record["createdAt"]),
	})
	if err != nil {
		writeError(w, err, trace)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cid":    cid,
		"txId":   res.TxID,
		"result": json.RawMessage(res.Payload),
		"trace":  trace.Steps,
	})
}

// accessRecord implementa a Figura 8.
func (s *Server) accessRecord(w http.ResponseWriter, r *http.Request) {
	trace := &Trace{}
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		Professional string `json:"professional"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	res, err := s.readRecord(r.Context(), org, r.PathValue("cid"), req.Professional, "", trace)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	res["trace"] = trace.Steps
	writeJSON(w, http.StatusOK, res)
}

// emergencyAccess implementa a Figura 9: registra a justificativa, que cria
// a permissão extraordinária de uso único, e realiza a leitura emergencial.
func (s *Server) emergencyAccess(w http.ResponseWriter, r *http.Request) {
	trace := &Trace{}
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		Professional string `json:"professional"`
		Reason       string `json:"reason"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	cid := r.PathValue("cid")
	record, err := s.ledger.Evaluate(org.Name, "readAsset", map[string]any{"key": recordRef(cid)}, nil)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	var rec struct {
		Patient map[string]any `json:"patient"`
	}
	if err := json.Unmarshal(record, &rec); err != nil {
		writeError(w, err, trace)
		return
	}

	em, err := s.ledger.Submit(org.Name, "requestEmergencyAccess", map[string]any{
		"patient":      keyRef(str(rec.Patient["@key"])),
		"professional": keyRef(req.Professional),
		"record":       recordRef(cid),
		"reason":       req.Reason,
	}, nil)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	trace.tx("exRecover", em.TxID, "justificativa registrada; permissão extraordinária de uso único, válida por 1 h")

	res, err := s.readRecord(r.Context(), org, cid, req.Professional, req.Reason, trace)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	res["emergencyTxId"] = em.TxID
	res["trace"] = trace.Steps
	writeJSON(w, http.StatusOK, res)
}

// readRecord solicita a leitura no ledger (auditada) e, se autorizada,
// recupera o envelope no IPFS, confere a integridade e decifra pelo KMS.
func (s *Server) readRecord(ctx context.Context, org config.Org, cid, profKey, reason string, trace *Trace) (map[string]any, error) {
	if profKey == "" {
		return nil, badRequest("informe o profissional")
	}
	res, err := s.ledger.Submit(org.Name, "requestRecordAccess",
		map[string]any{"record": recordRef(cid), "professional": keyRef(profKey)}, nil)
	if err != nil {
		return nil, err
	}
	var access struct {
		Granted     bool   `json:"granted"`
		Operation   string `json:"operation"`
		AccessLog   string `json:"accessLog"`
		Reason      string `json:"reason"`
		ContentHash string `json:"contentHash"`
		KMSKeyID    string `json:"kmsKeyId"`
	}
	if err := json.Unmarshal(res.Payload, &access); err != nil {
		return nil, err
	}
	out := map[string]any{
		"granted":   access.Granted,
		"operation": access.Operation,
		"accessLog": access.AccessLog,
		"txId":      res.TxID,
	}
	if !access.Granted {
		trace.tx("hasPermission", res.TxID, "acesso negado e auditado (%s): %s", access.Operation, access.Reason)
		out["reason"] = access.Reason
		return out, nil
	}
	trace.tx("hasPermission", res.TxID, "%s autorizada; registro de auditoria %s", access.Operation, access.AccessLog)

	// Confirmação exigida antes da decifração (getAccessAuthorization).
	authOut, err := s.ledger.Evaluate(org.Name, "getAccessAuthorization",
		map[string]any{"accessLog": keyRef(access.AccessLog)}, nil)
	if err != nil {
		return nil, err
	}
	var auth struct {
		Valid     bool   `json:"valid"`
		Reason    string `json:"reason"`
		Emergency bool   `json:"emergency"`
		Actor     string `json:"actor"`
	}
	if err := json.Unmarshal(authOut, &auth); err != nil {
		return nil, err
	}
	if !auth.Valid {
		return nil, &httpError{status: http.StatusForbidden, message: "autorização recusada: " + auth.Reason}
	}
	trace.add("Blockchain", "getAccessAuthorization", "registro corresponde a leitura autorizada, recente e da chave %s", access.KMSKeyID)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	envelope, err := s.ipfs[org.Name].Cat(ctx, cid)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(envelope)
	if hex.EncodeToString(sum[:]) != access.ContentHash {
		trace.add("IPFS", "getDocument", "SHA-256 divergente do registrado no ledger")
		return nil, &httpError{status: http.StatusBadGateway, message: "o envelope recuperado do IPFS não confere com o hash registrado"}
	}
	trace.add("IPFS", "getDocument", "envelope recuperado pelo nó de %s; SHA-256 confere com o ledger", org.Name)

	var rec kmsclient.EncryptedRecord
	if err := json.Unmarshal(envelope, &rec); err != nil {
		return nil, fmt.Errorf("envelope inválido: %w", err)
	}
	if auth.Emergency && reason == "" {
		reason = s.emergencyReason(org.Name, access.AccessLog)
	}
	inst, _ := s.dir.InstitutionOf(org.Name)
	prof, _ := s.dir.Professional(profKey)
	plaintext, err := s.kms.Decrypt(ctx, access.KMSKeyID, &rec, kmsclient.AuditInfo{
		RequesterID:   orDefault(prof.ID, profKey),
		InstitutionID: inst.ID,
		Emergency:     auth.Emergency,
		Reason:        reason,
	})
	if err != nil {
		return nil, err
	}
	trace.add("KMS", "decrypt", "%d decifrações parciais combinadas (limiar %d-de-%d); chave privada não reconstituída",
		s.kms.Threshold(), s.kms.Threshold(), s.kms.Total())

	var doc document
	if err := json.Unmarshal(plaintext, &doc); err != nil {
		return nil, fmt.Errorf("prontuário decifrado em formato inesperado: %w", err)
	}
	out["document"] = doc
	return out, nil
}

// emergencyReason recupera a justificativa do acesso extraordinário
// consumido na leitura, exigida pelos custodiantes para a auditoria local.
func (s *Server) emergencyReason(org, accessLog string) string {
	logAsset, err := s.readKey(org, accessLog)
	if err != nil {
		return "acesso extraordinário"
	}
	em, _ := logAsset["emergency"].(map[string]any)
	emAsset, err := s.readKey(org, str(em["@key"]))
	if err != nil || str(emAsset["reason"]) == "" {
		return "acesso extraordinário"
	}
	return str(emAsset["reason"])
}

// invalidateRecord realiza a eliminação lógica assinada pelo paciente e
// remove a fixação do envelope nos nós IPFS (Seção 5.3.2).
func (s *Server) invalidateRecord(w http.ResponseWriter, r *http.Request) {
	trace := &Trace{}
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		signed
		Reason string `json:"reason"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	cid := r.PathValue("cid")
	res, err := s.ledger.Submit(org.Name, "invalidateMedicalRecord", map[string]any{
		"record":    recordRef(cid),
		"reason":    req.Reason,
		"nonce":     req.Nonce,
		"signature": req.Signature,
	}, nil)
	if err != nil {
		writeError(w, err, trace)
		return
	}
	trace.tx("invalidateMedicalRecord", res.TxID, "eliminação lógica assinada pelo paciente; auditoria INVALIDACAO")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	for _, o := range s.cfg.Orgs {
		if err := s.ipfs[o.Name].Unpin(ctx, cid); err != nil {
			trace.add("IPFS", "unpin", "%s: %v", o.Name, err)
		} else {
			trace.add("IPFS", "unpin", "%s: fixação removida", o.Name)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": json.RawMessage(res.Payload), "txId": res.TxID, "trace": trace.Steps})
}

// allPatientRecords lista, para a carteira do paciente, os prontuários
// registrados pela API, com a situação lida do ledger.
func (s *Server) allPatientRecords(w http.ResponseWriter, r *http.Request) {
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	out := []map[string]any{}
	for _, rec := range s.dir.RecordsOf(r.PathValue("key")) {
		entry := map[string]any{
			"cid":        rec.CID,
			"key":        rec.Key,
			"title":      rec.Title,
			"recordType": rec.RecordType,
			"author":     rec.Author,
			"org":        rec.Org,
			"createdAt":  rec.CreatedAt,
		}
		raw, err := s.ledger.Evaluate(org.Name, "readAsset", map[string]any{"key": recordRef(rec.CID)}, nil)
		if err != nil {
			entry["error"] = err.Error()
		} else {
			var asset map[string]any
			_ = json.Unmarshal(raw, &asset)
			entry["key"] = asset["@key"]
			entry["invalidated"] = asset["invalidated"] == true
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, out)
}

func asFabric(err error, target **fabric.Error) bool {
	return errors.As(err, target)
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
