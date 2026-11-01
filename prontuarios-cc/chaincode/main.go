// Chaincode de controle de acesso a prontuários eletrônicos (B-02 a B-05),
// construído com o cc-tools e executado como serviço externo (CCaaS).
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hyperledger-labs/cc-tools/assets"
	"github.com/hyperledger-labs/cc-tools/events"
	tx "github.com/hyperledger-labs/cc-tools/transactions"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	pb "github.com/hyperledger/fabric-protos-go/peer"

	"prontuarios-cc/datatypes"
	"prontuarios-cc/header"
	"prontuarios-cc/txdefs"
)

var startupCheckExecuted = false

// SetupCC registra cabeçalho, tipos, ativos e transações no cc-tools.
func SetupCC() error {
	tx.InitHeader(tx.Header{
		Name:    header.Name,
		Version: header.Version,
		Colors:  header.Colors,
		Title:   header.Title,
	})

	assets.InitDynamicAssetTypeConfig(assets.DynamicAssetType{Enabled: false})

	tx.InitTxList(txList)

	if err := assets.CustomDataTypes(datatypes.CustomDataTypes); err != nil {
		return fmt.Errorf("registrando tipos de dados: %w", err)
	}
	assets.InitAssetList(assetTypeList)

	events.InitEventList([]events.Event{})

	return nil
}

func main() {
	log.Printf("Iniciando o chaincode %s versão %s\n", header.Name, header.Version)

	if err := txdefs.SetCPFPepper(os.Getenv("CPF_HASH_PEPPER")); err != nil {
		log.Fatalf("CPF_HASH_PEPPER: %s", err)
	}
	if err := SetupCC(); err != nil {
		log.Fatal(err)
	}

	var err error
	if os.Getenv("RUN_CCAAS") == "true" {
		err = runCCaaS()
	} else {
		err = shim.Start(new(ProntuariosCC))
	}
	if err != nil {
		log.Fatalf("Erro ao iniciar o chaincode: %s", err)
	}
}

// runCCaaS expõe o chaincode como servidor gRPC, conforme o builder ccaas
// instalado nos peers por fabric-network/scripts/provision.sh.
func runCCaaS() error {
	server := &shim.ChaincodeServer{
		CCID:    os.Getenv("CHAINCODE_ID"),
		Address: os.Getenv("CHAINCODE_SERVER_ADDRESS"),
		CC:      new(ProntuariosCC),
		TLSProps: shim.TLSProperties{
			Disabled: true,
		},
	}
	return server.Start()
}

// ProntuariosCC implementa shim.Chaincode.
type ProntuariosCC struct{}

// Init valida a configuração de ativos e transações na instanciação.
func (t *ProntuariosCC) Init(stub shim.ChaincodeStubInterface) (response pb.Response) {
	res := InitFunc(stub)
	startupCheckExecuted = true
	if res.Status != 200 {
		return res
	}
	return shim.Success(nil)
}

// InitFunc executa as verificações de consistência do cc-tools.
func InitFunc(stub shim.ChaincodeStubInterface) (response pb.Response) {
	defer logTx(stub, time.Now(), &response)

	if err := assets.StartupCheck(); err != nil {
		return err.GetErrorResponse()
	}
	if err := tx.StartupCheck(); err != nil {
		return err.GetErrorResponse()
	}
	return shim.Success(nil)
}

// Invoke executa uma transação.
func (t *ProntuariosCC) Invoke(stub shim.ChaincodeStubInterface) (response pb.Response) {
	defer logTx(stub, time.Now(), &response)

	if !startupCheckExecuted {
		res := InitFunc(stub)
		if res.Status != 200 {
			return res
		}
		startupCheckExecuted = true
	}

	result, err := tx.Run(stub)
	if err != nil {
		return err.GetErrorResponse()
	}
	return shim.Success(result)
}

func logTx(stub shim.ChaincodeStubInterface, beginTime time.Time, response *pb.Response) {
	fn, _ := stub.GetFunctionAndParameters()
	log.Printf("%d %s %s %s\n", response.Status, fn, time.Since(beginTime), response.Message)
}
