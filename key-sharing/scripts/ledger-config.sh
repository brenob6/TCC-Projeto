#!/usr/bin/env bash
#
# Prepara, no host, a conexão de cada custodiante com o livro-razão:
#
#   build/ledger/custodian-N/ledger.json  peer, MSP, canal e chaincode
#   build/ledger/custodian-N/tls-ca.crt   CA TLS do peer
#   build/ledger/custodian-N/cert.pem     identidade cliente do custodiante
#   build/ledger/custodian-N/key.pem      chave privada dessa identidade
#
# O custodiante N consulta o peer da organização OrgN com a identidade
# User2@orgN, emitida exclusivamente para ele (a API utiliza User1). Assim,
# cada custodiante confirma as autorizações em um peer distinto.
#
# Uso: scripts/ledger-config.sh <diretório organizations do fabric-network>

set -e

ORGANIZATIONS=$(cd "${1:?uso: $0 <diretório organizations>}" && pwd)
cd "$(dirname "$0")/.."

for i in 1 2 3; do
    domain=org${i}.example.com
    peer=peer0.${domain}
    user=${ORGANIZATIONS}/peerOrganizations/${domain}/users/User2@${domain}/msp
    out=build/ledger/custodian-${i}

    if [ ! -f ${user}/signcerts/User2@${domain}-cert.pem ]; then
        echo "ERRO: identidade User2@${domain} não encontrada." >&2
        echo "Execute fabric-network/scripts/generate.sh para gerá-la." >&2
        exit 1
    fi

    mkdir -p ${out}
    chmod 700 ${out}
    cp ${ORGANIZATIONS}/peerOrganizations/${domain}/peers/${peer}/tls/ca.crt ${out}/tls-ca.crt
    cp ${user}/signcerts/User2@${domain}-cert.pem ${out}/cert.pem
    cp ${user}/keystore/priv_sk ${out}/key.pem
    chmod 600 ${out}/key.pem
    cat > ${out}/ledger.json <<EOF
{
  "mspId": "Org${i}MSP",
  "peerAddress": "192.168.56.3${i}:7051",
  "peerHost": "${peer}",
  "tlsCaCert": "tls-ca.crt",
  "certPath": "cert.pem",
  "keyPath": "key.pem",
  "channel": "prontuarios",
  "chaincode": "prontuarios",
  "maxAgeSeconds": 300
}
EOF
    echo "custodian-${i}: peer ${peer} (Org${i}MSP), identidade User2@${domain}"
done
