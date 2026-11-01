#!/usr/bin/env bash
#
# Teste de ponta a ponta na rede Vagrant, executado no host após deploy.sh ou
# update.sh.
# Percorre os fluxos das Figuras 7 a 9 do TCC no ledger: cadastro,
# consentimento assinado, registro de prontuário, leitura autorizada,
# negativa, acesso extraordinário de uso único, revogação e histórico.
#
# O conteúdo do prontuário, o IPFS e o KMS não participam: o CID e o hash são
# simulados, pois a integração é objeto de B-06/B-09.

set -e

source "$(dirname "$0")/lib.sh"

REQ=${STAGE}/requests
SIGNER=${ROOT}/build/patient-signer
WORK=$(mktemp -d)
trap 'rm -rf ${WORK}' EXIT
mkdir -p ${REQ}

FAILED=0
SEQ=0

# cc <nó> <invoke|query> <transação> <args-json> [transiente-json]
cc() {
    local node=$1 mode=$2 fn=$3 args=$4 transient=$5
    SEQ=$((SEQ + 1))
    printf '%s' "${args}" > ${REQ}/${SEQ}.json
    local tfile=""
    if [ -n "${transient}" ]; then
        printf '%s' "${transient}" > ${REQ}/${SEQ}.t.json
        tfile=/vagrant/chaincode/requests/${SEQ}.t.json
    fi
    vm fabric-node-${node} \
        "bash /vagrant/chaincode/cc.sh ${mode} ${fn} /vagrant/chaincode/requests/${SEQ}.json ${tfile}"
}

# jget <json> <caminho.com.pontos>
jget() {
    python3 -I -c '
import json, sys
v = json.loads(sys.argv[1])
for p in sys.argv[2].split("."):
    v = v[int(p)] if isinstance(v, list) else v.get(p)
print(v if not isinstance(v, (dict, list, bool)) else json.dumps(v))
' "$1" "$2"
}

check() {
    local label=$1 got=$2 want=$3
    if [ "${got}" = "${want}" ]; then
        echo "  OK    ${label}"
    else
        echo "  FALHA ${label}: obtido '${got}', esperado '${want}'"
        FAILED=1
    fi
}

random_cpf() {
    python3 -I -c '
import random
d = [random.randint(0, 9) for _ in range(9)]
for n in (9, 10):
    s = sum(d[i] * (n + 1 - i) for i in range(n))
    r = 11 - s % 11
    d.append(0 if r > 9 else r)
print("".join(map(str, d)))
'
}

ensure_institution() {
    local node=$1 cnes=$2 name=$3 cnpj=$4
    cc ${node} invoke registerInstitution "{\"id\":\"${cnes}\",\"name\":\"${name}\",\"cnpj\":\"${cnpj}\"}" >/dev/null 2>&1 || true
    cc ${node} query readAsset "{\"key\":{\"@assetType\":\"institution\",\"id\":\"${cnes}\"}}"
}

echo "== Cadastro (B-02) =="
HA=$(ensure_institution 1 2345678 "Hospital A" 11222333000181)
HB=$(ensure_institution 2 3456789 "Hospital B" 45997418000153)
ensure_institution 3 4567890 "Clinica C" 06990590000123 >/dev/null
check "instituição de Org1" "$(jget "${HA}" mspId)" "Org1MSP"
check "instituição de Org2" "$(jget "${HB}" mspId)" "Org2MSP"

MA=$(cc 1 invoke registerProfessional \
    "{\"name\":\"Dra. Ana\",\"professionalType\":\"medico\",\"councilRegistry\":\"CRM-DF 1001\",\"institution\":{\"id\":\"2345678\"}}" \
    "{\"cpf\":\"$(random_cpf)\"}")
MB=$(cc 2 invoke registerProfessional \
    "{\"name\":\"Dr. Caio\",\"professionalType\":\"medico\",\"councilRegistry\":\"CRM-DF 3003\",\"institution\":{\"id\":\"3456789\"}}" \
    "{\"cpf\":\"$(random_cpf)\"}")
MA_KEY=$(jget "${MA}" @key)
MB_KEY=$(jget "${MB}" @key)

PATIENT_CPF=$(random_cpf)
SIGNING_KEY=$(${SIGNER} keygen ${WORK}/patient.key)
PATIENT=$(cc 1 invoke registerPatient \
    "{\"kmsKeyId\":\"paciente-e2e\",\"publicKey\":\"QzFfcHVibGljX2tleQ==\",\"signingPublicKey\":\"${SIGNING_KEY}\"}" \
    "{\"cpf\":\"${PATIENT_CPF}\"}")
PID=$(jget "${PATIENT}" id)
check "CPF ausente do ativo paciente" "$(echo "${PATIENT}" | grep -c "${PATIENT_CPF}" || true)" "0"
FOUND=$(cc 3 query getPatientByCpf '{}' "{\"cpf\":\"${PATIENT_CPF}\"}")
check "getPatientByCpf a partir de Org3" "$(jget "${FOUND}" id)" "${PID}"

echo "== Consentimento assinado (B-04) =="
GRANT_PAYLOAD="{\"professional\":\"$(jget "${MA}" id)\",\"institution\":\"\",\"accessLevel\":\"leituraEscrita\",\"purpose\":\"acompanhamento clinico\",\"records\":[],\"expiresAt\":\"\"}"
SIG=$(${SIGNER} sign ${WORK}/patient.key grantPermission ${PID} 1 "${GRANT_PAYLOAD}")
GRANT=$(cc 1 invoke grantPermission \
    "{\"patient\":{\"id\":\"${PID}\"},\"professional\":{\"@key\":\"${MA_KEY}\"},\"accessLevel\":\"leituraEscrita\",\"purpose\":\"acompanhamento clinico\",\"nonce\":1,\"signature\":\"${SIG}\"}")
PERM_ID=$(jget "${GRANT}" permission.id)
PERM_KEY=$(jget "${GRANT}" permission.@key)
check "permissão ativa" "$(jget "${GRANT}" permission.status)" "ativa"
if cc 1 invoke grantPermission \
    "{\"patient\":{\"id\":\"${PID}\"},\"professional\":{\"@key\":\"${MA_KEY}\"},\"accessLevel\":\"leituraEscrita\",\"purpose\":\"acompanhamento clinico\",\"nonce\":1,\"signature\":\"${SIG}\"}" >/dev/null 2>&1; then
    check "repetição da assinatura recusada" "aceita" "recusada"
else
    check "repetição da assinatura recusada" "recusada" "recusada"
fi

echo "== Registro do prontuário (Figura 7) =="
CID="bafy$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')"
HASH=$(head -c 64 /dev/urandom | sha256sum | cut -d' ' -f1)
REC=$(cc 1 invoke registerMedicalRecord \
    "{\"cid\":\"${CID}\",\"contentHash\":\"${HASH}\",\"patient\":{\"id\":\"${PID}\"},\"author\":{\"@key\":\"${MA_KEY}\"},\"recordType\":\"consulta\"}")
check "prontuário associado à chave do paciente" "$(jget "${REC}" record.kmsKeyId)" "paciente-e2e"

echo "== Recuperação (Figura 8) =="
READ=$(cc 1 invoke requestRecordAccess "{\"record\":{\"cid\":\"${CID}\"},\"professional\":{\"@key\":\"${MA_KEY}\"}}")
check "leitura autorizada" "$(jget "${READ}" granted)" "true"
AUTH=$(cc 2 query getAccessAuthorization "{\"accessLog\":{\"@key\":\"$(jget "${READ}" accessLog)\"}}")
check "autorização válida para o KMS" "$(jget "${AUTH}" valid)" "true"
check "chave informada ao KMS" "$(jget "${AUTH}" kmsKeyId)" "paciente-e2e"

DENIED=$(cc 2 invoke requestRecordAccess "{\"record\":{\"cid\":\"${CID}\"},\"professional\":{\"@key\":\"${MB_KEY}\"}}")
check "leitura sem permissão negada e registrada" "$(jget "${DENIED}" operation)" "ACESSO_NEGADO"

echo "== Recuperação extraordinária (Figura 9) =="
EM=$(cc 2 invoke requestEmergencyAccess \
    "{\"patient\":{\"id\":\"${PID}\"},\"professional\":{\"@key\":\"${MB_KEY}\"},\"record\":{\"cid\":\"${CID}\"},\"reason\":\"paciente inconsciente no pronto-socorro\"}")
check "acesso extraordinário ativo" "$(jget "${EM}" emergencyAccess.status)" "ativa"
EREAD=$(cc 2 invoke requestRecordAccess "{\"record\":{\"cid\":\"${CID}\"},\"professional\":{\"@key\":\"${MB_KEY}\"}}")
check "leitura emergencial" "$(jget "${EREAD}" operation)" "LEITURA_EMERGENCIAL"
EREAD2=$(cc 2 invoke requestRecordAccess "{\"record\":{\"cid\":\"${CID}\"},\"professional\":{\"@key\":\"${MB_KEY}\"}}")
check "uso único do acesso extraordinário" "$(jget "${EREAD2}" granted)" "false"

echo "== Revogação (RF-04) =="
SIG=$(${SIGNER} sign ${WORK}/patient.key revokePermission ${PID} 2 "{\"permission\":\"${PERM_ID}\"}")
cc 1 invoke revokePermission "{\"permission\":{\"@key\":\"${PERM_KEY}\"},\"nonce\":2,\"signature\":\"${SIG}\"}" >/dev/null
AFTER=$(cc 1 invoke requestRecordAccess "{\"record\":{\"cid\":\"${CID}\"},\"professional\":{\"@key\":\"${MA_KEY}\"}}")
check "leitura após revogação negada" "$(jget "${AFTER}" granted)" "false"

echo "== Histórico de acessos (RF-08) =="
HIST=$(cc 3 query getAccessHistory "{\"patient\":{\"id\":\"${PID}\"}}")
OPS=$(python3 -I -c 'import json,sys; print(",".join(h["operation"] for h in json.loads(sys.argv[1])))' "${HIST}")
check "sequência auditada" "${OPS}" \
    "CONCESSAO,CRIACAO,LEITURA,ACESSO_NEGADO,SOLICITACAO_EMERGENCIAL,LEITURA_EMERGENCIAL,ACESSO_NEGADO,REVOGACAO,ACESSO_NEGADO"

rm -rf ${REQ}
echo ""
if [ ${FAILED} -ne 0 ]; then
    echo "Teste de ponta a ponta: FALHOU"
    exit 1
fi
echo "Teste de ponta a ponta: OK"
