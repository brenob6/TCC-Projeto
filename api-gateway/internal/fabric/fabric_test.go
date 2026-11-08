package fabric

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTranslateChaincodeError(t *testing.T) {
	st, err := status.New(codes.Aborted, "failed to endorse transaction").WithDetails(&gateway.ErrorDetail{
		Address: "peer0.org1.example.com:7051",
		MspId:   "Org1MSP",
		Message: "chaincode response 403, o paciente não concedeu permissão de escrita ao autor",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := translate(st.Err())
	fe, ok := got.(*Error)
	if !ok || fe.Status != 403 || fe.Message != "o paciente não concedeu permissão de escrita ao autor" {
		t.Fatalf("tradução inesperada: %#v", got)
	}
}

func TestTranslateUnavailable(t *testing.T) {
	got := translate(status.Error(codes.Unavailable, "connection refused"))
	if fe, ok := got.(*Error); !ok || fe.Status != 502 {
		t.Fatalf("tradução inesperada: %#v", got)
	}
}
