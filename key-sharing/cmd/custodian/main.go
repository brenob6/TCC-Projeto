// Comando custodian executa um Custodiante de Chaves.
//
// Uso:
//
//	custodian -id custodian-1 -listen 0.0.0.0:50051 -db /var/lib/custodian/shares.db
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"poc-key-sharing/internal/custodian"
	"poc-key-sharing/pb"
)

func main() {
	id := flag.String("id", hostname(), "identificador do custodiante")
	listen := flag.String("listen", "0.0.0.0:50051", "endereço de escuta gRPC")
	dbPath := flag.String("db", "shares.db", "caminho da base SQLite local")
	flag.Parse()

	store, err := custodian.OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("erro ao abrir base local: %v", err)
	}
	defer store.Close()

	lis, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("erro ao escutar em %s: %v", *listen, err)
	}

	server := grpc.NewServer()
	pb.RegisterCustodianServiceServer(server, custodian.NewServer(*id, store))

	// Encerramento ordenado: as conexões em curso são concluídas antes de sair.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		log.Printf("[%s] encerrando...", *id)
		server.GracefulStop()
	}()

	log.Printf("[%s] custodiante escutando em %s (base: %s)", *id, *listen, *dbPath)
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
