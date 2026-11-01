package main

import (
	"github.com/hyperledger-labs/cc-tools/assets"

	"prontuarios-cc/assettypes"
)

var assetTypeList = []assets.AssetType{
	assettypes.Institution,
	assettypes.Patient,
	assettypes.Professional,
	assettypes.MedicalRecord,
	assettypes.AccessPermission,
	assettypes.EmergencyAccess,
	assettypes.AccessLog,
}
