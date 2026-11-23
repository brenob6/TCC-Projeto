// Comando api inicia a API Gateway da arquitetura (Figura 6): recebe as
// requisições do frontend e coordena a rede Fabric, a rede IPFS privada e o
// KMS com os custodiantes de chaves.
package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"poc-key-sharing/kmsclient"

	"api-gateway/internal/config"
	"api-gateway/internal/directory"
	"api-gateway/internal/fabric"
	"api-gateway/internal/ipfs"
	"api-gateway/internal/server"
)

func main() {
	configPath := flag.String("config", "config.json", "arquivo de configuração (opcional)")
	organizations := flag.String("organizations", "../fabric-network/organizations",
		"material criptográfico gerado por fabric-network/scripts/generate.sh")
	listen := flag.String("listen", "", "endereço HTTP (padrão 127.0.0.1:8080)")
	static := flag.String("static", "", "diretório do build do frontend, servido em /")
	flag.Parse()

	cfg, err := config.Load(*configPath, config.Default(*organizations))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal(err)
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *static != "" {
		cfg.StaticDir = *static
	}

	ledger, err := fabric.Connect(cfg)
	if err != nil {
		log.Fatalf("Fabric: %v", err)
	}
	defer ledger.Close()

	storages := map[string]server.Storage{}
	for _, org := range cfg.Orgs {
		storages[org.Name] = ipfs.New(org.IPFSAPI)
	}

	custodianTLS, err := kmsclient.ClientTLS(cfg.CustodianTLS)
	if err != nil {
		log.Fatalf("KMS: %v", err)
	}
	keys, err := kmsclient.New(kmsclient.Config{
		Threshold:  cfg.Threshold,
		Custodians: cfg.Custodians,
		Timeout:    10 * time.Second,
		TLS:        custodianTLS,
	})
	if err != nil {
		log.Fatalf("KMS: %v", err)
	}
	defer keys.Close()

	dir, err := directory.Open(cfg.DirectoryFile)
	if err != nil {
		log.Fatalf("diretório: %v", err)
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           server.New(cfg, ledger, storages, keys, dir).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("API Gateway em http://%s (canal %s, chaincode %s)", cfg.Listen, cfg.Channel, cfg.Chaincode)
	log.Fatal(srv.ListenAndServe())
}
