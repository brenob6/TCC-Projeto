// Comando patient-signer simula a carteira do paciente: gera o par de chaves
// Ed25519 e assina as mensagens canônicas exigidas por grantPermission,
// revokePermission e invalidateMedicalRecord.
//
//	patient-signer keygen <arquivo-da-chave>
//	patient-signer sign <arquivo-da-chave> <ação> <id-do-paciente> <nonce> <payload-json>
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"prontuarios-cc/txdefs"
)

func main() {
	if len(os.Args) < 3 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2])
	case "sign":
		if len(os.Args) != 7 {
			usage()
		}
		err = sign(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "uso: patient-signer keygen <chave> | sign <chave> <ação> <paciente> <nonce> <payload-json>")
	os.Exit(2)
}

// keygen grava a semente privada (Base64) e imprime a chave pública.
func keygen(path string) error {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(priv.Seed())), 0o600); err != nil {
		return err
	}
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	return nil
}

func sign(path, action, patient, nonceStr, payloadJSON string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	seed, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil || len(seed) != ed25519.SeedSize {
		return fmt.Errorf("chave inválida em %s", path)
	}
	nonce, err := strconv.ParseInt(nonceStr, 10, 64)
	if err != nil {
		return fmt.Errorf("nonce inválido: %w", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}

	msg, err := txdefs.SignedMessage(action, patient, nonce, payload)
	if err != nil {
		return err
	}
	fmt.Println(base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(seed), msg)))
	return nil
}
