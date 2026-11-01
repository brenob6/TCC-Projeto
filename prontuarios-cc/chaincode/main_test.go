package main

import (
	"container/list"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/hyperledger-labs/cc-tools/mock"

	"prontuarios-cc/txdefs"
)

func TestMain(m *testing.M) {
	log.SetFlags(log.Lshortfile)

	if err := txdefs.SetCPFPepper("segredo-de-teste-do-consorcio"); err != nil {
		log.Fatal(err)
	}
	if err := SetupCC(); err != nil {
		log.Fatal(err)
	}

	stub := mock.NewMockStub("Org1MSP", new(ProntuariosCC))
	if res := stub.MockInit("init", [][]byte{[]byte("init")}); res.GetStatus() != 200 {
		log.Fatal(res.GetMessage())
	}

	os.Exit(m.Run())
}

// network simula o canal: um MockStub por organização, todos sobre o mesmo
// estado.
type network struct {
	t     *testing.T
	stubs map[string]*mock.MockStub
	state map[string][]byte
	keys  *list.List
	txSeq int
}

func newNetwork(t *testing.T) *network {
	n := &network{
		t:     t,
		stubs: map[string]*mock.MockStub{},
		state: map[string][]byte{},
		keys:  list.New(),
	}
	return n
}

func (n *network) stub(msp string) *mock.MockStub {
	s, ok := n.stubs[msp]
	if !ok {
		s = mock.NewMockStub(msp, new(ProntuariosCC))
		s.State = n.state
		s.Keys = n.keys
		n.stubs[msp] = s
	}
	return s
}

type response struct {
	status  int32
	message string
	payload []byte
}

func (r response) object(t *testing.T) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(r.payload, &m); err != nil {
		t.Fatalf("resposta não é um objeto JSON: %s (%s)", r.payload, err)
	}
	return m
}

func (r response) list(t *testing.T) []map[string]interface{} {
	t.Helper()
	var l []map[string]interface{}
	if err := json.Unmarshal(r.payload, &l); err != nil {
		t.Fatalf("resposta não é uma lista JSON: %s (%s)", r.payload, err)
	}
	return l
}

// invoke executa uma transação como msp. transient vai em @request.
func (n *network) invoke(msp, fn string, args, transient map[string]interface{}) response {
	n.t.Helper()
	s := n.stub(msp)
	s.TransientMap = nil
	if transient != nil {
		raw, _ := json.Marshal(transient)
		s.TransientMap = map[string][]byte{"@request": raw}
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	raw, _ := json.Marshal(args)
	n.txSeq++
	res := s.MockInvoke(fmt.Sprintf("tx-%04d", n.txSeq), [][]byte{[]byte(fn), raw})
	return response{status: res.GetStatus(), message: res.GetMessage(), payload: res.GetPayload()}
}

// mustInvoke falha o teste se a transação não retornar 200.
func (n *network) mustInvoke(msp, fn string, args, transient map[string]interface{}) response {
	n.t.Helper()
	res := n.invoke(msp, fn, args, transient)
	if res.status != 200 {
		n.t.Fatalf("%s como %s: status %d: %s", fn, msp, res.status, res.message)
	}
	return res
}

func (n *network) expectStatus(status int32, msp, fn string, args, transient map[string]interface{}) response {
	n.t.Helper()
	res := n.invoke(msp, fn, args, transient)
	if res.status != status {
		n.t.Fatalf("%s como %s: esperado status %d, obtido %d: %s", fn, msp, status, res.status, res.message)
	}
	return res
}

// patientWallet guarda o par de chaves do paciente e o próximo nonce.
type patientWallet struct {
	id    string
	priv  ed25519.PrivateKey
	nonce int64
}

// sign devolve os argumentos nonce e signature para a ação.
func (w *patientWallet) sign(t *testing.T, action string, payload map[string]interface{}) map[string]interface{} {
	t.Helper()
	w.nonce++
	msg, err := txdefs.SignedMessage(action, w.id, w.nonce, payload)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]interface{}{
		"nonce":     w.nonce,
		"signature": base64.StdEncoding.EncodeToString(ed25519.Sign(w.priv, msg)),
	}
}

func merge(maps ...map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// Fixture com três instituições, profissionais e um paciente.
const (
	cpfPaciente   = "529.982.247-25"
	cpfMedicoA    = "111.444.777-35"
	cpfEnfermeiro = "123.456.789-09"
	cpfMedicoB    = "987.654.321-00"
)

type fixture struct {
	*network
	patient    *patientWallet
	medicoA    map[string]interface{} // Org1, médico
	enfermeiro map[string]interface{} // Org1, enfermeiro
	medicoB    map[string]interface{} // Org2, médico
	hospitalA  map[string]interface{}
	hospitalB  map[string]interface{}
}

func ref(key string) map[string]interface{} {
	return map[string]interface{}{"@key": key}
}

func newFixture(t *testing.T) *fixture {
	n := newNetwork(t)
	f := &fixture{network: n}

	f.hospitalA = n.mustInvoke("Org1MSP", "registerInstitution", map[string]interface{}{
		"id": "2345678", "name": "Hospital A", "cnpj": "11.222.333/0001-81",
	}, nil).object(t)
	f.hospitalB = n.mustInvoke("Org2MSP", "registerInstitution", map[string]interface{}{
		"id": "3456789", "name": "Hospital B", "cnpj": "45.997.418/0001-53",
	}, nil).object(t)
	n.mustInvoke("Org3MSP", "registerInstitution", map[string]interface{}{
		"id": "4567890", "name": "Clínica C", "cnpj": "06.990.590/0001-23",
	}, nil)

	f.medicoA = f.registerProfessional("Org1MSP", f.hospitalA, cpfMedicoA, "Dra. Ana", "medico", "CRM-DF 1001")
	f.enfermeiro = f.registerProfessional("Org1MSP", f.hospitalA, cpfEnfermeiro, "Enf. Bruno", "enfermeiro", "COREN-DF 2002")
	f.medicoB = f.registerProfessional("Org2MSP", f.hospitalB, cpfMedicoB, "Dr. Caio", "medico", "CRM-DF 3003")

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	patient := n.mustInvoke("Org1MSP", "registerPatient", map[string]interface{}{
		"kmsKeyId":         "paciente-001",
		"publicKey":        "QzFfcHVibGljX2tleQ==",
		"signingPublicKey": base64.StdEncoding.EncodeToString(pub),
	}, map[string]interface{}{"cpf": cpfPaciente}).object(t)
	f.patient = &patientWallet{id: patient["id"].(string), priv: priv}

	return f
}

func (f *fixture) registerProfessional(msp string, inst map[string]interface{}, cpf, name, kind, registry string) map[string]interface{} {
	return f.mustInvoke(msp, "registerProfessional", map[string]interface{}{
		"name":             name,
		"professionalType": kind,
		"councilRegistry":  registry,
		"institution":      ref(inst["@key"].(string)),
	}, map[string]interface{}{"cpf": cpf}).object(f.t)
}

func (f *fixture) patientRef() map[string]interface{} {
	return map[string]interface{}{"id": f.patient.id}
}

// grant concede uma permissão assinada pelo paciente. grantee deve conter
// "professional" ou "institution" (o "id" do ativo).
func (f *fixture) grant(msp string, grantee map[string]interface{}, level string, records []string, extra map[string]interface{}) response {
	f.t.Helper()
	payload := map[string]interface{}{
		"professional": "",
		"institution":  "",
		"accessLevel":  level,
		"purpose":      "acompanhamento clínico",
		"records":      append([]string{}, records...),
		"expiresAt":    "",
	}
	args := map[string]interface{}{
		"patient":     f.patientRef(),
		"accessLevel": level,
		"purpose":     "acompanhamento clínico",
	}
	for k, v := range grantee {
		payload[k] = v
		args[k] = map[string]interface{}{"id": v}
	}
	if len(records) > 0 {
		refs := []interface{}{}
		for _, cid := range records {
			refs = append(refs, map[string]interface{}{"cid": cid})
		}
		args["records"] = refs
	}
	for k, v := range extra {
		args[k] = v
		payload[k] = v
	}
	return f.invoke(msp, "grantPermission", merge(args, f.patient.sign(f.t, "grantPermission", payload)), nil)
}

func (f *fixture) registerRecord(msp string, author map[string]interface{}, cid string) response {
	f.t.Helper()
	return f.invoke(msp, "registerMedicalRecord", map[string]interface{}{
		"cid":         cid,
		"contentHash": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		"patient":     f.patientRef(),
		"author":      ref(author["@key"].(string)),
		"recordType":  "consulta",
	}, nil)
}

func (f *fixture) readRecord(msp string, prof map[string]interface{}, cid string) map[string]interface{} {
	f.t.Helper()
	return f.mustInvoke(msp, "requestRecordAccess", map[string]interface{}{
		"record":       map[string]interface{}{"cid": cid},
		"professional": ref(prof["@key"].(string)),
	}, nil).object(f.t)
}

// setState altera diretamente uma propriedade gravada, para simular a passagem
// do tempo.
func (f *fixture) setState(key, prop string, value interface{}) {
	f.t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(f.state[key], &m); err != nil {
		f.t.Fatal(err)
	}
	m[prop] = value
	raw, _ := json.Marshal(m)
	f.state[key] = raw
}
