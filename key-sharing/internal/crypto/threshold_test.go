package crypto

import (
	"bytes"
	"encoding/json"
	"testing"
)

const (
	testThreshold = 2
	testTotal     = 3
)

// partialsFor produz as decifrações parciais dos shares indicados, simulando o
// trabalho de cada custodiante.
func partialsFor(t *testing.T, shares []Share, c1 []byte, indices ...int) []Partial {
	t.Helper()

	var partials []Partial
	for _, i := range indices {
		value, err := PartialDecrypt(shares[i].Value, c1)
		if err != nil {
			t.Fatalf("decifração parcial do share %d: %v", i, err)
		}
		partials = append(partials, Partial{Index: shares[i].Index, Value: value})
	}
	return partials
}

func TestEncryptDecryptComLimiarAtingido(t *testing.T) {
	pubKey, shares, err := GenerateKey(testThreshold, testTotal)
	if err != nil {
		t.Fatalf("gerando chave: %v", err)
	}

	prontuario := []byte("Paciente 001 - hipertensão arterial")
	rec, err := Encrypt(pubKey, prontuario)
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}

	if bytes.Contains(rec.Ciphertext, prontuario) {
		t.Error("o texto claro aparece no prontuário cifrado")
	}

	partials := partialsFor(t, shares, rec.C1, 0, 1)
	got, err := CombineAndDecrypt(rec, partials, testThreshold, testTotal)
	if err != nil {
		t.Fatalf("decifrando: %v", err)
	}
	if !bytes.Equal(got, prontuario) {
		t.Errorf("prontuário recuperado = %q, esperado %q", got, prontuario)
	}
}

// Qualquer subconjunto de k custodiantes deve recuperar o prontuário, o que
// garante tolerância à indisponibilidade dos demais.
func TestQualquerSubconjuntoDeTamanhoK(t *testing.T) {
	pubKey, shares, err := GenerateKey(testThreshold, testTotal)
	if err != nil {
		t.Fatalf("gerando chave: %v", err)
	}

	prontuario := []byte("Paciente 002 - diabetes tipo II")
	rec, err := Encrypt(pubKey, prontuario)
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}

	subconjuntos := [][]int{{0, 1}, {0, 2}, {1, 2}}
	for _, indices := range subconjuntos {
		partials := partialsFor(t, shares, rec.C1, indices...)
		got, err := CombineAndDecrypt(rec, partials, testThreshold, testTotal)
		if err != nil {
			t.Fatalf("decifrando com shares %v: %v", indices, err)
		}
		if !bytes.Equal(got, prontuario) {
			t.Errorf("shares %v: recuperado %q, esperado %q", indices, got, prontuario)
		}
	}
}

// Abaixo do limiar a decifração deve ser recusada: é a propriedade central do
// esquema (k,n).
func TestAbaixoDoLimiarFalha(t *testing.T) {
	pubKey, shares, err := GenerateKey(testThreshold, testTotal)
	if err != nil {
		t.Fatalf("gerando chave: %v", err)
	}

	rec, err := Encrypt(pubKey, []byte("Paciente 003 - alergia a penicilina"))
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}

	partials := partialsFor(t, shares, rec.C1, 0)
	if _, err := CombineAndDecrypt(rec, partials, testThreshold, testTotal); err == nil {
		t.Error("decifração com um único share deveria falhar")
	}
}

// Um share de outra chave não pode decifrar o prontuário, ainda que o limiar
// numérico seja atingido.
func TestShareDeOutraChaveFalha(t *testing.T) {
	pubKey, shares, err := GenerateKey(testThreshold, testTotal)
	if err != nil {
		t.Fatalf("gerando chave: %v", err)
	}
	_, outros, err := GenerateKey(testThreshold, testTotal)
	if err != nil {
		t.Fatalf("gerando segunda chave: %v", err)
	}

	rec, err := Encrypt(pubKey, []byte("Paciente 004 - histórico cirúrgico"))
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}

	partials := append(
		partialsFor(t, shares, rec.C1, 0),
		partialsFor(t, outros, rec.C1, 1)...,
	)
	if _, err := CombineAndDecrypt(rec, partials, testThreshold, testTotal); err == nil {
		t.Error("decifração com share de outra chave deveria falhar")
	}
}

func TestGenerateKeyRejeitaLimiarInvalido(t *testing.T) {
	casos := []struct {
		nome      string
		threshold int
		total     int
	}{
		{"limiar menor que dois", 1, 3},
		{"limiar maior que o total", 4, 3},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, _, err := GenerateKey(c.threshold, c.total); err == nil {
				t.Errorf("GenerateKey(%d, %d) deveria falhar", c.threshold, c.total)
			}
		})
	}
}

// O prontuário cifrado deve ser autenticado: adulterá-lo invalida a decifração.
func TestCiphertextAdulteradoFalha(t *testing.T) {
	pubKey, shares, err := GenerateKey(testThreshold, testTotal)
	if err != nil {
		t.Fatalf("gerando chave: %v", err)
	}

	rec, err := Encrypt(pubKey, []byte("Paciente 005 - exames laboratoriais"))
	if err != nil {
		t.Fatalf("cifrando: %v", err)
	}
	rec.Ciphertext[0] ^= 0xFF

	partials := partialsFor(t, shares, rec.C1, 0, 1)
	if _, err := CombineAndDecrypt(rec, partials, testThreshold, testTotal); err == nil {
		t.Error("decifração de prontuário adulterado deveria falhar")
	}
}

// O envelope serializado em JSON, como armazenado no IPFS, é decodificado sem
// perda, e envelopes malformados ou sem C1 são recusados.
func TestParseEnvelope(t *testing.T) {
	pubKey, _, err := GenerateKey(2, 3)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := Encrypt(pubKey, []byte("prontuário"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseEnvelope(data)
	if err != nil {
		t.Fatalf("decodificando envelope: %v", err)
	}
	if !bytes.Equal(got.C1, rec.C1) || !bytes.Equal(got.Ciphertext, rec.Ciphertext) {
		t.Fatal("envelope decodificado difere do original")
	}

	for _, invalido := range []string{`não é JSON`, `{"ciphertext": "AA=="}`} {
		if _, err := ParseEnvelope([]byte(invalido)); err == nil {
			t.Errorf("envelope %q deveria ser recusado", invalido)
		}
	}
}
