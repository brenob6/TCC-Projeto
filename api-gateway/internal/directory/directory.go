// Package directory guarda, fora da blockchain, os dados que a interface de
// demonstração precisa para listar participantes: o ledger não oferece
// listagens gerais e não armazena o nome do paciente. Em um sistema real,
// esses dados ficariam no sistema de prontuário de cada instituição.
//
// O CPF não é gravado aqui: a localização por CPF usa getPatientByCpf.
package directory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Institution é a instituição de uma organização.
type Institution struct {
	Org  string `json:"org"`
	Key  string `json:"key"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Professional é um profissional cadastrado por uma organização.
type Professional struct {
	Org              string `json:"org"`
	Key              string `json:"key"`
	ID               string `json:"id"`
	Name             string `json:"name"`
	ProfessionalType string `json:"professionalType"`
	CouncilRegistry  string `json:"councilRegistry"`
}

// Patient associa o identificador pseudônimo ao nome exibido.
type Patient struct {
	Key      string `json:"key"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Org      string `json:"org"`
	KMSKeyID string `json:"kmsKeyId"`
}

// Record é a referência a um prontuário registrado pela API.
type Record struct {
	CID        string `json:"cid"`
	Key        string `json:"key"`
	Patient    string `json:"patient"`
	Author     string `json:"author"`
	Org        string `json:"org"`
	RecordType string `json:"recordType"`
	Title      string `json:"title"`
	CreatedAt  string `json:"createdAt"`
}

// Data é o conteúdo persistido.
type Data struct {
	Institutions  []Institution  `json:"institutions"`
	Professionals []Professional `json:"professionals"`
	Patients      []Patient      `json:"patients"`
	Records       []Record       `json:"records"`
}

// Store persiste o diretório em um arquivo JSON.
type Store struct {
	mu   sync.RWMutex
	path string
	data Data
}

// Open carrega o arquivo, se existir.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, err
	}
	return s, nil
}

// Snapshot devolve uma cópia do conteúdo.
func (s *Store) Snapshot() Data {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Data{
		Institutions:  append([]Institution{}, s.data.Institutions...),
		Professionals: append([]Professional{}, s.data.Professionals...),
		Patients:      append([]Patient{}, s.data.Patients...),
		Records:       append([]Record{}, s.data.Records...),
	}
}

// Update aplica fn ao conteúdo e grava o arquivo.
func (s *Store) Update(fn func(*Data)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.data)
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// InstitutionOf devolve a instituição da organização.
func (s *Store) InstitutionOf(org string) (Institution, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, i := range s.data.Institutions {
		if i.Org == org {
			return i, true
		}
	}
	return Institution{}, false
}

// PutInstitution grava ou substitui a instituição da organização.
func (s *Store) PutInstitution(inst Institution) error {
	return s.Update(func(d *Data) {
		for i := range d.Institutions {
			if d.Institutions[i].Org == inst.Org {
				d.Institutions[i] = inst
				return
			}
		}
		d.Institutions = append(d.Institutions, inst)
	})
}

// AddProfessional inclui o profissional, se ainda não existir.
func (s *Store) AddProfessional(p Professional) error {
	return s.Update(func(d *Data) {
		for _, e := range d.Professionals {
			if e.Key == p.Key {
				return
			}
		}
		d.Professionals = append(d.Professionals, p)
	})
}

// AddPatient inclui o paciente, se ainda não existir.
func (s *Store) AddPatient(p Patient) error {
	return s.Update(func(d *Data) {
		for _, e := range d.Patients {
			if e.Key == p.Key {
				return
			}
		}
		d.Patients = append(d.Patients, p)
	})
}

// AddRecord inclui a referência ao prontuário.
func (s *Store) AddRecord(r Record) error {
	return s.Update(func(d *Data) { d.Records = append(d.Records, r) })
}

// Professional busca um profissional pela chave.
func (s *Store) Professional(key string) (Professional, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.data.Professionals {
		if p.Key == key {
			return p, true
		}
	}
	return Professional{}, false
}

// RecordsOf lista os prontuários do paciente registrados pela API.
func (s *Store) RecordsOf(patientKey string) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Record{}
	for _, r := range s.data.Records {
		if r.Patient == patientKey {
			out = append(out, r)
		}
	}
	return out
}

// Record busca um prontuário pelo CID.
func (s *Store) Record(cid string) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.data.Records {
		if r.CID == cid {
			return r, true
		}
	}
	return Record{}, false
}
