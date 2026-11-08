// Package config define a topologia usada pela API Gateway. Os valores
// padrão correspondem aos ambientes Vagrant de ../fabric-network (peers e
// nós IPFS) e ../key-sharing (custodiantes).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Org descreve a conexão de uma instituição (organização Fabric) com o seu
// peer e o seu nó IPFS.
type Org struct {
	Name        string `json:"name"`        // rótulo exibido na interface (Org1)
	MSPID       string `json:"mspId"`       // Org1MSP
	PeerAddress string `json:"peerAddress"` // 192.168.56.31:7051
	PeerHost    string `json:"peerHost"`    // nome no certificado TLS do peer
	TLSCACert   string `json:"tlsCaCert"`   // CA TLS do peer
	CertPath    string `json:"certPath"`    // certificado da identidade cliente
	KeyPath     string `json:"keyPath"`     // chave privada da identidade cliente
	IPFSAPI     string `json:"ipfsApi"`     // http://192.168.56.31:5001
}

// Config reúne a configuração da API.
type Config struct {
	Listen        string   `json:"listen"`
	Channel       string   `json:"channel"`
	Chaincode     string   `json:"chaincode"`
	Orgs          []Org    `json:"orgs"`
	Custodians    []string `json:"custodians"`
	Threshold     int      `json:"threshold"`
	DirectoryFile string   `json:"directoryFile"`
	StaticDir     string   `json:"staticDir"`
}

// Default monta a configuração da rede Vagrant a partir do diretório do
// material criptográfico gerado por fabric-network/scripts/generate.sh.
func Default(organizations string) Config {
	cfg := Config{
		Listen:    "127.0.0.1:8080",
		Channel:   "prontuarios",
		Chaincode: "prontuarios",
		Custodians: []string{
			"192.168.56.21:50051",
			"192.168.56.22:50051",
			"192.168.56.23:50051",
		},
		Threshold:     2,
		DirectoryFile: "data/directory.json",
	}
	for i := 1; i <= 3; i++ {
		domain := fmt.Sprintf("org%d.example.com", i)
		peer := "peer0." + domain
		user := filepath.Join(organizations, "peerOrganizations", domain, "users", "User1@"+domain, "msp")
		cfg.Orgs = append(cfg.Orgs, Org{
			Name:        fmt.Sprintf("Org%d", i),
			MSPID:       fmt.Sprintf("Org%dMSP", i),
			PeerAddress: fmt.Sprintf("192.168.56.3%d:7051", i),
			PeerHost:    peer,
			TLSCACert:   filepath.Join(organizations, "peerOrganizations", domain, "peers", peer, "tls", "ca.crt"),
			CertPath:    filepath.Join(user, "signcerts", "User1@"+domain+"-cert.pem"),
			KeyPath:     filepath.Join(user, "keystore", "priv_sk"),
			IPFSAPI:     fmt.Sprintf("http://192.168.56.3%d:5001", i),
		})
	}
	return cfg
}

// Load lê um arquivo JSON sobre os valores padrão. Campos ausentes mantêm o
// valor padrão.
func Load(path string, base Config) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return base, err
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return base, fmt.Errorf("lendo %s: %w", path, err)
	}
	return base, nil
}

// Org devolve a organização pelo nome.
func (c Config) Org(name string) (Org, bool) {
	for _, o := range c.Orgs {
		if o.Name == name {
			return o, true
		}
	}
	return Org{}, false
}
