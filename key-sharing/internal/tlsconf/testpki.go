package tlsconf

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"time"
)

// TestPKI é uma autoridade certificadora efêmera, mantida apenas em memória,
// usada pelos testes automatizados para estabelecer TLS mútuo entre o
// coordenador e custodiantes locais.
type TestPKI struct {
	Server   *tls.Config // configuração dos custodiantes (127.0.0.1)
	Client   *tls.Config // configuração do coordenador
	Intruder *tls.Config // cliente com certificado de outra CA
}

// NewTestPKI gera a CA, o certificado dos custodiantes, o do coordenador e o
// de um cliente não autorizado, emitido por uma CA distinta.
func NewTestPKI() (*TestPKI, error) {
	ca, caKey, err := newCA("CA de teste")
	if err != nil {
		return nil, err
	}
	otherCA, otherKey, err := newCA("CA desconhecida")
	if err != nil {
		return nil, err
	}

	server, err := issue(ca, caKey, "custodiante", x509.ExtKeyUsageServerAuth)
	if err != nil {
		return nil, err
	}
	client, err := issue(ca, caKey, "coordenador", x509.ExtKeyUsageClientAuth)
	if err != nil {
		return nil, err
	}
	intruder, err := issue(otherCA, otherKey, "intruso", x509.ExtKeyUsageClientAuth)
	if err != nil {
		return nil, err
	}

	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return &TestPKI{
		Server: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    pool,
			Certificates: []tls.Certificate{server},
		},
		Client: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			RootCAs:      pool,
			Certificates: []tls.Certificate{client},
		},
		Intruder: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			RootCAs:      pool,
			Certificates: []tls.Certificate{intruder},
		},
	}, nil
}

func newCA(name string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	return cert, key, err
}

func issue(ca *x509.Certificate, caKey *ecdsa.PrivateKey, name string, usage x509.ExtKeyUsage) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
