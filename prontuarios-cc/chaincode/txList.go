package main

import (
	tx "github.com/hyperledger-labs/cc-tools/transactions"

	"prontuarios-cc/txdefs"
)

// txList não inclui createAsset, updateAsset e deleteAsset: toda escrita passa
// pelas transações abaixo, que aplicam as regras de negócio e a auditoria. O
// cc-tools acrescenta as consultas básicas (readAsset, readAssetHistory,
// getSchema etc.).
var txList = []tx.Transaction{
	// B-02 – Registro de usuários
	txdefs.RegisterInstitution,
	txdefs.SetInstitutionStatus,
	txdefs.RegisterPatient,
	txdefs.GetPatientByCpf,
	txdefs.RegisterProfessional,
	txdefs.SetProfessionalStatus,

	// B-03 – Registro e recuperação de prontuários
	txdefs.RegisterMedicalRecord,
	txdefs.RequestRecordAccess,
	txdefs.GetAccessAuthorization,
	txdefs.ListPatientRecords,
	txdefs.GetAccessHistory,
	txdefs.InvalidateMedicalRecord,

	// B-04 – Controle de acesso
	txdefs.GrantPermission,
	txdefs.RevokePermission,
	txdefs.ListPermissions,
	txdefs.CheckPermission,

	// B-05 – Recuperação extraordinária
	txdefs.RequestEmergencyAccess,
	txdefs.ListEmergencyAccesses,
}
