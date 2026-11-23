// Package tlsconf monta as configurações de TLS mútuo entre o coordenador do
// KMS e os custodiantes. Os dois lados apresentam certificados emitidos pela
// mesma autoridade certificadora: o custodiante só aceita conexões de um
// coordenador autenticado, e o coordenador só confia em custodiantes cujo
// certificado corresponda ao endereço configurado.
package tlsconf

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// Files reúne os caminhos do material TLS de uma das partes.
type Files struct {
	CACert string `json:"caCert"`   // CA que emitiu os certificados das duas partes
	Cert   string `json:"certPath"` // certificado da própria parte
	Key    string `json:"keyPath"`  // chave privada da própria parte
}

// Client devolve a configuração TLS do coordenador do KMS.
func Client(f Files) (*tls.Config, error) {
	pool, cert, err := load(f)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
	}, nil
}

// Server devolve a configuração TLS do custodiante, que exige e verifica o
// certificado do cliente.
func Server(f Files) (*tls.Config, error) {
	pool, cert, err := load(f)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		Certificates: []tls.Certificate{cert},
	}, nil
}

func load(f Files) (*x509.CertPool, tls.Certificate, error) {
	if f.CACert == "" || f.Cert == "" || f.Key == "" {
		return nil, tls.Certificate{}, fmt.Errorf("TLS: informe a CA, o certificado e a chave privada")
	}
	caPEM, err := os.ReadFile(f.CACert)
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("lendo a CA TLS: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, tls.Certificate{}, fmt.Errorf("CA TLS inválida em %s", f.CACert)
	}
	cert, err := tls.LoadX509KeyPair(f.Cert, f.Key)
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("lendo o certificado TLS: %w", err)
	}
	return pool, cert, nil
}
