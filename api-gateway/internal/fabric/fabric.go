// Package fabric conecta a API ao chaincode prontuarios com a identidade de
// cada organização, por meio do Fabric Gateway do peer da instituição.
package fabric

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/hash"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"api-gateway/internal/config"
)

// Result é a resposta de uma transação.
type Result struct {
	TxID    string
	Payload json.RawMessage
}

// Error traduz a resposta de erro do chaincode (cc-tools) para a API.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// Client mantém uma conexão por organização.
type Client struct {
	conns     map[string]*grpc.ClientConn
	gateways  map[string]*client.Gateway
	contracts map[string]*client.Contract
}

// Connect abre as conexões com os peers de todas as organizações.
func Connect(cfg config.Config) (*Client, error) {
	c := &Client{
		conns:     map[string]*grpc.ClientConn{},
		gateways:  map[string]*client.Gateway{},
		contracts: map[string]*client.Contract{},
	}
	for _, org := range cfg.Orgs {
		if err := c.connectOrg(cfg, org); err != nil {
			c.Close()
			return nil, fmt.Errorf("%s: %w", org.Name, err)
		}
	}
	return c, nil
}

func (c *Client) connectOrg(cfg config.Config, org config.Org) error {
	caPEM, err := os.ReadFile(org.TLSCACert)
	if err != nil {
		return fmt.Errorf("lendo a CA TLS do peer: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("CA TLS inválida em %s", org.TLSCACert)
	}
	conn, err := grpc.NewClient(org.PeerAddress,
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(pool, org.PeerHost)))
	if err != nil {
		return fmt.Errorf("conectando ao peer: %w", err)
	}
	c.conns[org.Name] = conn

	certPEM, err := os.ReadFile(org.CertPath)
	if err != nil {
		return fmt.Errorf("lendo o certificado da identidade: %w", err)
	}
	cert, err := identity.CertificateFromPEM(certPEM)
	if err != nil {
		return err
	}
	id, err := identity.NewX509Identity(org.MSPID, cert)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(org.KeyPath)
	if err != nil {
		return fmt.Errorf("lendo a chave da identidade: %w", err)
	}
	key, err := identity.PrivateKeyFromPEM(keyPEM)
	if err != nil {
		return err
	}
	sign, err := identity.NewPrivateKeySign(key)
	if err != nil {
		return err
	}

	gw, err := client.Connect(id,
		client.WithSign(sign),
		client.WithHash(hash.SHA256),
		client.WithClientConnection(conn),
		client.WithEvaluateTimeout(15*time.Second),
		client.WithEndorseTimeout(30*time.Second),
		client.WithSubmitTimeout(15*time.Second),
		client.WithCommitStatusTimeout(60*time.Second),
	)
	if err != nil {
		return err
	}
	c.gateways[org.Name] = gw
	c.contracts[org.Name] = gw.GetNetwork(cfg.Channel).GetContract(cfg.Chaincode)
	return nil
}

// Close encerra as conexões.
func (c *Client) Close() {
	for _, gw := range c.gateways {
		gw.Close()
	}
	for _, conn := range c.conns {
		conn.Close()
	}
}

func (c *Client) contract(org string) (*client.Contract, error) {
	contract, ok := c.contracts[org]
	if !ok {
		return nil, &Error{Status: 400, Message: fmt.Sprintf("organização desconhecida: %s", org)}
	}
	return contract, nil
}

// options monta os argumentos no formato do cc-tools: um único JSON com os
// argumentos públicos e, no mapa transiente @request, os argumentos privados.
func options(args, transient any) ([]client.ProposalOption, error) {
	if args == nil {
		args = map[string]any{}
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	opts := []client.ProposalOption{client.WithBytesArguments(argsJSON)}
	if transient != nil {
		tJSON, err := json.Marshal(transient)
		if err != nil {
			return nil, err
		}
		opts = append(opts, client.WithTransient(map[string][]byte{"@request": tJSON}))
	}
	return opts, nil
}

// Evaluate executa uma consulta no peer da organização, sem gravar no ledger.
func (c *Client) Evaluate(org, fn string, args, transient any) (json.RawMessage, error) {
	contract, err := c.contract(org)
	if err != nil {
		return nil, err
	}
	opts, err := options(args, transient)
	if err != nil {
		return nil, err
	}
	out, err := contract.Evaluate(fn, opts...)
	if err != nil {
		return nil, translate(err)
	}
	return out, nil
}

// Submit endossa, ordena e aguarda a confirmação de uma transação. O Fabric
// Gateway do peer coleta os endossos exigidos pela política (MAJORITY).
func (c *Client) Submit(org, fn string, args, transient any) (*Result, error) {
	contract, err := c.contract(org)
	if err != nil {
		return nil, err
	}
	opts, err := options(args, transient)
	if err != nil {
		return nil, err
	}
	proposal, err := contract.NewProposal(fn, opts...)
	if err != nil {
		return nil, err
	}
	tx, err := proposal.Endorse()
	if err != nil {
		return nil, translate(err)
	}
	commit, err := tx.Submit()
	if err != nil {
		return nil, translate(err)
	}
	st, err := commit.Status()
	if err != nil {
		return nil, translate(err)
	}
	if !st.Successful {
		return nil, &Error{Status: 409, Message: fmt.Sprintf("transação %s rejeitada na validação: %s", st.TransactionID, st.Code)}
	}
	return &Result{TxID: tx.TransactionID(), Payload: tx.Result()}, nil
}

// chaincodeResponse extrai status e mensagem de "chaincode response 403, ...".
var chaincodeResponse = regexp.MustCompile(`chaincode response (\d{3}), (.*)`)

// translate converte o erro do gateway na mensagem devolvida pelo chaincode.
func translate(err error) error {
	// Os erros do Fabric Gateway expõem GRPCStatus(), com os detalhes de
	// cada peer.
	st, ok := status.FromError(err)
	if !ok {
		return &Error{Status: 502, Message: err.Error()}
	}
	messages := []string{st.Message()}
	for _, d := range st.Details() {
		if detail, ok := d.(interface{ GetMessage() string }); ok {
			messages = append(messages, detail.GetMessage())
		}
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if m := chaincodeResponse.FindStringSubmatch(messages[i]); m != nil {
			code, _ := strconv.Atoi(m[1])
			return &Error{Status: code, Message: m[2]}
		}
	}
	return &Error{Status: 502, Message: st.Message()}
}
