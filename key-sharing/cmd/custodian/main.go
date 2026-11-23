// Comando custodian executa um Custodiante de Chaves.
//
// Uso:
//
//	custodian -id custodian-1 -listen 0.0.0.0:50051 -db /var/lib/custodian/shares.db \
//	    -tls-ca ca.crt -tls-cert custodian-1.crt -tls-key custodian-1.key \
//	    -ledger /etc/custodian/ledger/ledger.json
//
// Com -ledger, toda decifração parcial exige a confirmação da autorização no
// livro-razão. A opção -isolado dispensa essa confirmação e destina-se apenas
// aos testes do módulo sem a rede blockchain; uma das duas é obrigatória.
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"poc-key-sharing/internal/custodian"
	"poc-key-sharing/internal/ledger"
	"poc-key-sharing/internal/tlsconf"
	"poc-key-sharing/pb"
)

func main() {
	id := flag.String("id", hostname(), "identificador do custodiante")
	listen := flag.String("listen", "0.0.0.0:50051", "endereço de escuta gRPC")
	dbPath := flag.String("db", "shares.db", "caminho da base SQLite local")
	tlsCA := flag.String("tls-ca", "", "CA que emitiu o certificado do coordenador do KMS")
	tlsCert := flag.String("tls-cert", "", "certificado TLS do custodiante")
	tlsKey := flag.String("tls-key", "", "chave privada TLS do custodiante")
	ledgerConfig := flag.String("ledger", "", "configuração da conexão com o peer (JSON)")
	isolated := flag.Bool("isolado", false, "dispensa a verificação no livro-razão (apenas testes do módulo)")
	flag.Parse()

	if (*ledgerConfig == "") == !*isolated {
		log.Fatalf("informe -ledger <arquivo> ou -isolado (apenas um dos dois)")
	}

	tlsCfg, err := tlsconf.Server(tlsconf.Files{CACert: *tlsCA, Cert: *tlsCert, Key: *tlsKey})
	if err != nil {
		log.Fatalf("configurando TLS: %v", err)
	}

	store, err := custodian.OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("erro ao abrir base local: %v", err)
	}
	defer store.Close()

	var auth custodian.Authorizer
	if *ledgerConfig != "" {
		cfg, err := ledger.Load(*ledgerConfig)
		if err != nil {
			log.Fatalf("lendo a configuração do livro-razão: %v", err)
		}
		client, err := ledger.Connect(cfg)
		if err != nil {
			log.Fatalf("conectando ao peer %s: %v", cfg.PeerAddress, err)
		}
		defer client.Close()
		auth = client
		log.Printf("[%s] autorizações confirmadas no livro-razão pelo peer %s (%s)", *id, cfg.PeerAddress, cfg.MSPID)
	} else {
		log.Printf("[%s] AVISO: modo isolado; as decifrações não são confirmadas no livro-razão", *id)
	}

	lis, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("erro ao escutar em %s: %v", *listen, err)
	}

	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsCfg)))
	pb.RegisterCustodianServiceServer(server, custodian.NewServer(*id, store, auth))

	// Encerramento ordenado: as conexões em curso são concluídas antes de sair.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		log.Printf("[%s] encerrando...", *id)
		server.GracefulStop()
	}()

	log.Printf("[%s] custodiante escutando em %s com TLS mútuo (base: %s)", *id, *listen, *dbPath)
	if err := server.Serve(lis); err != nil {
		log.Fatalf("erro no servidor gRPC: %v", err)
	}
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "custodian"
	}
	return h
}
