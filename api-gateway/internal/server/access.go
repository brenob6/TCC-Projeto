package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) listPatientRecords(w http.ResponseWriter, r *http.Request) {
	s.evaluate(w, r, "listPatientRecords", func(r *http.Request) (map[string]any, error) {
		prof := r.URL.Query().Get("professional")
		if prof == "" {
			return nil, badRequest("informe o profissional")
		}
		return map[string]any{"patient": keyRef(r.PathValue("key")), "professional": keyRef(prof)}, nil
	})
}

func (s *Server) listPermissions(w http.ResponseWriter, r *http.Request) {
	s.evaluate(w, r, "listPermissions", func(r *http.Request) (map[string]any, error) {
		return map[string]any{"patient": keyRef(r.PathValue("key"))}, nil
	})
}

func (s *Server) checkPermission(w http.ResponseWriter, r *http.Request) {
	s.evaluate(w, r, "checkPermission", func(r *http.Request) (map[string]any, error) {
		prof := r.URL.Query().Get("professional")
		if prof == "" {
			return nil, badRequest("informe o profissional")
		}
		args := map[string]any{"patient": keyRef(r.PathValue("key")), "professional": keyRef(prof)}
		if cid := r.URL.Query().Get("record"); cid != "" {
			args["record"] = recordRef(cid)
		}
		return args, nil
	})
}

func (s *Server) accessHistory(w http.ResponseWriter, r *http.Request) {
	s.evaluate(w, r, "getAccessHistory", func(r *http.Request) (map[string]any, error) {
		args := map[string]any{"patient": keyRef(r.PathValue("key"))}
		q := r.URL.Query()
		if cid := q.Get("record"); cid != "" {
			args["record"] = recordRef(cid)
		}
		for _, p := range []string{"from", "to"} {
			if v := q.Get(p); v != "" {
				args[p] = v
			}
		}
		return args, nil
	})
}

func (s *Server) listEmergencies(w http.ResponseWriter, r *http.Request) {
	s.evaluate(w, r, "listEmergencyAccesses", func(r *http.Request) (map[string]any, error) {
		return map[string]any{"patient": keyRef(r.PathValue("key"))}, nil
	})
}

// evaluate executa uma consulta cujos argumentos são montados por build.
func (s *Server) evaluate(w http.ResponseWriter, r *http.Request, fn string, build func(*http.Request) (map[string]any, error)) {
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	args, err := build(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	out, err := s.ledger.Evaluate(org.Name, fn, args, nil)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, json.RawMessage(out))
}

// signed reúne os campos comuns às operações assinadas pelo paciente. A
// assinatura é produzida na carteira do paciente (navegador); a API apenas a
// encaminha e não consegue forjá-la.
type signed struct {
	Nonce     int64  `json:"nonce"`
	Signature string `json:"signature"`
}

func (s *Server) grantPermission(w http.ResponseWriter, r *http.Request) {
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req struct {
		signed
		Professional string   `json:"professional"`
		Institution  string   `json:"institution"`
		AccessLevel  string   `json:"accessLevel"`
		Purpose      string   `json:"purpose"`
		Records      []string `json:"records"`
		ExpiresAt    string   `json:"expiresAt"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	args := map[string]any{
		"patient":     keyRef(r.PathValue("key")),
		"accessLevel": req.AccessLevel,
		"purpose":     req.Purpose,
		"nonce":       req.Nonce,
		"signature":   req.Signature,
	}
	if req.Professional != "" {
		args["professional"] = keyRef(req.Professional)
	}
	if req.Institution != "" {
		args["institution"] = keyRef(req.Institution)
	}
	if len(req.Records) > 0 {
		refs := []any{}
		for _, cid := range req.Records {
			refs = append(refs, recordRef(cid))
		}
		args["records"] = refs
	}
	if req.ExpiresAt != "" {
		args["expiresAt"] = req.ExpiresAt
	}
	res, err := s.ledger.Submit(org.Name, "grantPermission", args, nil)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res.Payload, "txId": res.TxID})
}

func (s *Server) revokePermission(w http.ResponseWriter, r *http.Request) {
	org, err := s.org(r)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	var req signed
	if err := decode(r, &req); err != nil {
		writeError(w, err, nil)
		return
	}
	res, err := s.ledger.Submit(org.Name, "revokePermission", map[string]any{
		"permission": keyRef(r.PathValue("key")),
		"nonce":      req.Nonce,
		"signature":  req.Signature,
	}, nil)
	if err != nil {
		writeError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res.Payload, "txId": res.TxID})
}
