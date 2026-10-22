// Comando kms exercita os fluxos de operação do sistema de compartilhamento de
// chaves sobre os custodiantes distribuídos.
//
// Uso:
//
//	kms -custodians host1:50051,host2:50051,host3:50051 -k 2 <comando> [args]
//
// Comandos:
//
//	health                              Consulta o estado dos custodiantes
//	keygen <key-id>                     Gera a chave e distribui os shares
//	encrypt <key-id> <arquivo>          Cifra um prontuário
//	decrypt <key-id> <arquivo.enc>      Recupera o prontuário (fluxo autorizado)
//	emergency <key-id> <arquivo.enc>    Recuperação extraordinária
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"poc-key-sharing/internal/crypto"
	"poc-key-sharing/internal/kms"
)

// keyDir guarda as chaves públicas dos pacientes. Somente material público é
// persistido: os compartilhamentos ficam com os custodiantes.
const keyDir = "keys"

func main() {
	custodians := flag.String("custodians", envOr("CUSTODIANS", "localhost:50051"),
		"endereços gRPC dos custodiantes, separados por vírgula")
	threshold := flag.Int("k", envIntOr("THRESHOLD", 2), "limiar k do esquema (k,n)")
	requester := flag.String("requester", "medico-001", "identificador do solicitante")
	institution := flag.String("institution", "hospital-a", "instituição de origem")
	reason := flag.String("reason", "", "justificativa (obrigatória na recuperação extraordinária)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	addrs := splitAddrs(*custodians)
	service, err := kms.New(kms.Config{
		Threshold:  *threshold,
		Custodians: addrs,
		Timeout:    10 * time.Second,
	})
	if err != nil {
		fatal(err)
	}
	defer service.Close()

	ctx := context.Background()
	switch args[0] {
	case "health":
		err = health(ctx, service)
	case "keygen":
		err = withArgs(args, 2, "keygen <key-id>", func() error {
			return keygen(ctx, service, args[1])
		})
	case "encrypt":
		err = withArgs(args, 3, "encrypt <key-id> <arquivo>", func() error {
			return encrypt(service, args[1], args[2])
		})
	case "decrypt":
		err = withArgs(args, 3, "decrypt <key-id> <arquivo.enc>", func() error {
			return decrypt(ctx, service, args[1], args[2], kms.AuditInfo{
				RequesterID:   *requester,
				InstitutionID: *institution,
			})
		})
	case "emergency":
		err = withArgs(args, 3, "emergency <key-id> <arquivo.enc>", func() error {
			justification := *reason
			if justification == "" {
				justification = "atendimento emergencial: paciente inconsciente"
			}
			return decrypt(ctx, service, args[1], args[2], kms.AuditInfo{
				RequesterID:   *requester,
				InstitutionID: *institution,
				Emergency:     true,
				Reason:        justification,
			})
		})
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		fatal(err)
	}
}

func health(ctx context.Context, service *kms.KMS) error {
	statuses := service.HealthCheck(ctx)

	addrs := make([]string, 0, len(statuses))
	for addr := range statuses {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	fmt.Printf("Esquema %d-de-%d\n", service.Threshold(), service.Total())
	for _, addr := range addrs {
		fmt.Printf("  %-24s %s\n", addr, statuses[addr])
	}
	return nil
}

func keygen(ctx context.Context, service *kms.KMS, keyID string) error {
	pubKey, err := service.GenerateAndDistribute(ctx, keyID)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(keyDir, 0o755); err != nil {
		return fmt.Errorf("criando diretório %s: %w", keyDir, err)
	}
	path := publicKeyPath(keyID)
	if err := writeJSON(path, map[string][]byte{"point": pubKey}); err != nil {
		return err
	}

	fmt.Printf("Chave %s criada no esquema %d-de-%d\n", keyID, service.Threshold(), service.Total())
	fmt.Printf("Chave pública salva em %s\n", path)
	fmt.Println("A chave privada não foi reconstituída nem persistida.")
	return nil
}

func encrypt(service *kms.KMS, keyID, inputPath string) error {
	var pk struct {
		Point []byte `json:"point"`
	}
	if err := readJSON(publicKeyPath(keyID), &pk); err != nil {
		return fmt.Errorf("carregando chave pública de %s: %w", keyID, err)
	}

	plaintext, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("lendo prontuário: %w", err)
	}

	rec, err := service.Encrypt(pk.Point, plaintext)
	if err != nil {
		return err
	}

	outputPath := inputPath + ".enc"
	if err := writeJSON(outputPath, rec); err != nil {
		return err
	}

	fmt.Printf("Prontuário cifrado em %s\n", outputPath)
	fmt.Println("A chave simétrica não foi persistida em disco.")
	return nil
}

func decrypt(ctx context.Context, service *kms.KMS, keyID, inputPath string, audit kms.AuditInfo) error {
	var rec crypto.EncryptedRecord
	if err := readJSON(inputPath, &rec); err != nil {
		return fmt.Errorf("carregando prontuário cifrado: %w", err)
	}

	if audit.Emergency {
		fmt.Printf("Recuperação extraordinária — justificativa: %s\n", audit.Reason)
	}

	plaintext, err := service.Decrypt(ctx, keyID, &rec, audit)
	if err != nil {
		return err
	}

	outputPath := strings.TrimSuffix(inputPath, ".enc")
	if outputPath == inputPath {
		outputPath = inputPath + ".dec"
	}
	outputPath = "decifrado_" + filepath.Base(outputPath)
	if err := os.WriteFile(outputPath, plaintext, 0o600); err != nil {
		return fmt.Errorf("gravando prontuário decifrado: %w", err)
	}

	fmt.Printf("Prontuário recuperado em %s\n", outputPath)
	return nil
}

// --- Utilitários ---

func publicKeyPath(keyID string) string {
	return filepath.Join(keyDir, keyID+".pub.json")
}

func withArgs(args []string, n int, form string, fn func() error) error {
	if len(args) < n {
		return fmt.Errorf("uso: kms [flags] %s", form)
	}
	return fn()
}

func splitAddrs(s string) []string {
	var addrs []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			addrs = append(addrs, trimmed)
		}
	}
	return addrs
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("gravando %s: %w", path, err)
	}
	return nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	var n int
	if _, err := fmt.Sscanf(os.Getenv(key), "%d", &n); err == nil && n > 0 {
		return n
	}
	return fallback
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
	os.Exit(1)
}

func usage() {
	fmt.Println(`KMS — compartilhamento de chaves com custodiantes via gRPC

Uso:
  kms [flags] <comando> [args]

Comandos:
  health                            Consulta o estado dos custodiantes
  keygen <key-id>                   Gera a chave e distribui os shares
  encrypt <key-id> <arquivo>        Cifra um prontuário
  decrypt <key-id> <arquivo.enc>    Recupera o prontuário (fluxo autorizado)
  emergency <key-id> <arquivo.enc>  Recuperação extraordinária

Flags:
  -custodians  endereços gRPC separados por vírgula (env CUSTODIANS)
  -k           limiar k do esquema (k,n) (env THRESHOLD)
  -requester   identificador do solicitante
  -institution instituição de origem
  -reason      justificativa da recuperação extraordinária

Exemplo:
  kms -custodians 192.168.56.21:50051,192.168.56.22:50051,192.168.56.23:50051 -k 2 \
      keygen paciente-001`)
}
