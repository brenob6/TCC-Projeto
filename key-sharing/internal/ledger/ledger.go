// Package ledger permite que o custodiante confirme, no livro-razão, que um
// registro de auditoria autoriza a decifração. A consulta é feita pelo Fabric
// Gateway do peer de uma organização do consórcio, com uma identidade cliente
// exclusiva do custodiante, pela transação somente leitura
// getAccessAuthorization do chaincode prontuarios.
package ledger

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/hash"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"poc-key-sharing/internal/custodian"
)

// Config descreve a conexão do custodiante com o peer. Caminhos relativos
// são resolvidos a partir do diretório do arquivo de configuração.
type Config struct {
	MSPID         string `json:"mspId"`         // Org1MSP
	PeerAddress   string `json:"peerAddress"`   // 192.168.56.31:7051
	PeerHost      string `json:"peerHost"`      // nome no certificado TLS do peer
	TLSCACert     string `json:"tlsCaCert"`     // CA TLS do peer
	CertPath      string `json:"certPath"`      // certificado da identidade cliente
	KeyPath       string `json:"keyPath"`       // chave privada da identidade cliente
	Channel       string `json:"channel"`       // prontuarios
	Chaincode     string `json:"chaincode"`     // prontuarios
	MaxAgeSeconds int    `json:"maxAgeSeconds"` // idade máxima do registro de leitura
}

// Load lê a configuração em JSON.
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("lendo %s: %w", path, err)
	}
	dir := filepath.Dir(path)
	for _, p := range []*string{&cfg.TLSCACert, &cfg.CertPath, &cfg.KeyPath} {
		if *p != "" && !filepath.IsAbs(*p) {
			*p = filepath.Join(dir, *p)
		}
	}
	if cfg.Channel == "" {
		cfg.Channel = "prontuarios"
	}
	if cfg.Chaincode == "" {
		cfg.Chaincode = "prontuarios"
	}
	if cfg.MSPID == "" || cfg.PeerAddress == "" {
		return cfg, fmt.Errorf("%s: mspId e peerAddress são obrigatórios", path)
	}
	return cfg, nil
}

// Client consulta o chaincode com a identidade do custodiante.
type Client struct {
	conn     *grpc.ClientConn
	gateway  *client.Gateway
	contract *client.Contract
	maxAge   int
}

// Connect abre a conexão com o peer.
func Connect(cfg Config) (*Client, error) {
	caPEM, err := os.ReadFile(cfg.TLSCACert)
	if err != nil {
		return nil, fmt.Errorf("lendo a CA TLS do peer: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("CA TLS inválida em %s", cfg.TLSCACert)
	}
	conn, err := grpc.NewClient(cfg.PeerAddress,
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(pool, cfg.PeerHost)))
	if err != nil {
		return nil, fmt.Errorf("conectando ao peer: %w", err)
	}

	id, sign, err := loadIdentity(cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	gw, err := client.Connect(id,
		client.WithSign(sign),
		client.WithHash(hash.SHA256),
		client.WithClientConnection(conn),
		client.WithEvaluateTimeout(10*time.Second),
	)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &Client{
		conn:     conn,
		gateway:  gw,
		contract: gw.GetNetwork(cfg.Channel).GetContract(cfg.Chaincode),
		maxAge:   cfg.MaxAgeSeconds,
	}, nil
}

func loadIdentity(cfg Config) (*identity.X509Identity, identity.Sign, error) {
	certPEM, err := os.ReadFile(cfg.CertPath)
	if err != nil {
		return nil, nil, fmt.Errorf("lendo o certificado da identidade: %w", err)
	}
	cert, err := identity.CertificateFromPEM(certPEM)
	if err != nil {
		return nil, nil, err
	}
	id, err := identity.NewX509Identity(cfg.MSPID, cert)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := os.ReadFile(cfg.KeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("lendo a chave da identidade: %w", err)
	}
	key, err := identity.PrivateKeyFromPEM(keyPEM)
	if err != nil {
		return nil, nil, err
	}
	sign, err := identity.NewPrivateKeySign(key)
	if err != nil {
		return nil, nil, err
	}
	return id, sign, nil
}

// Close encerra a conexão.
func (c *Client) Close() {
	c.gateway.Close()
	c.conn.Close()
}

// Authorize executa getAccessAuthorization para o registro de auditoria.
func (c *Client) Authorize(ctx context.Context, accessLog string) (*custodian.Authorization, error) {
	args := map[string]any{"accessLog": keyRef(accessLog)}
	if c.maxAge > 0 {
		args["maxAgeSeconds"] = c.maxAge
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	proposal, err := c.contract.NewProposal("getAccessAuthorization", client.WithBytesArguments(argsJSON))
	if err != nil {
		return nil, err
	}
	out, err := proposal.EvaluateWithContext(ctx)
	if err != nil {
		// Uma recusa do chaincode (registro inexistente, por exemplo) é uma
		// resposta do livro-razão; os demais erros indicam que ele não pôde
		// ser consultado.
		if reason, ok := chaincodeRefusal(err); ok {
			return &custodian.Authorization{Reason: reason}, nil
		}
		return nil, err
	}
	return parse(out)
}

var chaincodeResponse = regexp.MustCompile(`chaincode response (\d{3}), (.*)`)

// chaincodeRefusal extrai a resposta 4xx do chaincode dos detalhes do erro do
// Fabric Gateway.
func chaincodeRefusal(err error) (string, bool) {
	st, ok := status.FromError(err)
	if !ok {
		return "", false
	}
	messages := []string{st.Message()}
	for _, d := range st.Details() {
		if detail, ok := d.(interface{ GetMessage() string }); ok {
			messages = append(messages, detail.GetMessage())
		}
	}
	for _, m := range messages {
		if sub := chaincodeResponse.FindStringSubmatch(m); sub != nil {
			if code, _ := strconv.Atoi(sub[1]); code >= 400 && code < 500 {
				return sub[2], true
			}
		}
	}
	return "", false
}

// keyRef monta a referência a um ativo no formato do cc-tools.
func keyRef(key string) map[string]any {
	assetType, _, _ := strings.Cut(key, ":")
	return map[string]any{"@assetType": assetType, "@key": key}
}

// parse interpreta a resposta da transação getAccessAuthorization.
func parse(out []byte) (*custodian.Authorization, error) {
	var res struct {
		Valid           bool   `json:"valid"`
		Reason          string `json:"reason"`
		KMSKeyID        string `json:"kmsKeyId"`
		ContentHash     string `json:"contentHash"`
		Actor           string `json:"actor"`
		Institution     string `json:"institution"`
		Emergency       bool   `json:"emergency"`
		EmergencyReason string `json:"emergencyReason"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("resposta inesperada do chaincode: %w", err)
	}
	return &custodian.Authorization{
		Valid:           res.Valid,
		Reason:          res.Reason,
		KMSKeyID:        res.KMSKeyID,
		ContentHash:     res.ContentHash,
		Actor:           res.Actor,
		Institution:     res.Institution,
		Emergency:       res.Emergency,
		EmergencyReason: res.EmergencyReason,
	}, nil
}
