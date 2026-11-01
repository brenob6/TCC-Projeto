#!/usr/bin/env bash
#
# Submete uma transação ao chaincode a partir desta máquina, com a identidade
# do administrador da organização local.
#
#   cc.sh invoke|query <transação> <arquivo-args.json> [arquivo-transiente.json]
#
# Imprime o payload JSON da resposta. Os argumentos vão em arquivos para
# evitar problemas de escape ao chamar via "vagrant ssh -c".

set -e

source /vagrant/chaincode/common.sh

MODE=$1
FN=$2
ARGS=$(cat "$3")
TRANSIENT=()
if [ -n "$4" ]; then
    TRANSIENT=(--transient "{\"@request\":\"$(base64 -w0 < "$4")\"}")
fi

CTOR=$(python3 -c 'import json,sys; print(json.dumps({"Args": [sys.argv[1], sys.argv[2]]}))' "${FN}" "${ARGS}")

if [ "${MODE}" = "query" ]; then
    peer chaincode query -C ${CHANNEL_NAME} -n ${CC_NAME} -c "${CTOR}" "${TRANSIENT[@]}"
    exit 0
fi

# Endosso pelo peer local e por um segundo peer (MAJORITY de três).
INDEX=${CORE_PEER_LOCALMSPID//[!0-9]/}
OTHER=$(( INDEX % 3 + 1 ))
OUT=$(peer chaincode invoke -o ${ORDERER_ADDRESS} --tls --cafile ${ORDERER_CA} \
    -C ${CHANNEL_NAME} -n ${CC_NAME} -c "${CTOR}" "${TRANSIENT[@]}" \
    --peerAddresses peer0.org${INDEX}.example.com:7051 --tlsRootCertFiles $(peer_tls_ca ${INDEX}) \
    --peerAddresses peer0.org${OTHER}.example.com:7051 --tlsRootCertFiles $(peer_tls_ca ${OTHER}) \
    --waitForEvent 2>&1) || { echo "${OUT}" >&2; exit 1; }

# O CLI imprime o payload no formato texto do protobuf (string entre aspas,
# com escapes octais para bytes não ASCII); converte de volta para UTF-8.
python3 - "${OUT}" <<'PY'
import codecs, re, sys
out = sys.argv[1]
m = re.search(r'payload:"((?:[^"\\]|\\.)*)"', out)
if not m:
    sys.stderr.write(out + "\n")
    sys.exit(1)
print(codecs.escape_decode(m.group(1).encode())[0].decode("utf-8"))
PY
