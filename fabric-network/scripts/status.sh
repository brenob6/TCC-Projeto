#!/usr/bin/env bash
#
# Exibe, a partir do host, o estado do canal em cada máquina: participação e
# papel de cada orderer no Raft e altura do ledger em cada peer. Somente
# consultas; nenhuma transação é submetida.

set -e

cd "$(dirname "$0")/.."

for NODE in fabric-node-1 fabric-node-2 fabric-node-3; do
    echo ""
    echo "--------------------------------------"
    echo "${NODE}"
    echo "--------------------------------------"
    vagrant ssh ${NODE} -c '
        source /etc/profile.d/fabric.sh
        echo "[orderer ${ORDERER_ADDRESS}]"
        osnadmin channel list --channelID ${CHANNEL_NAME} \
            -o ${ORDERER_ADMIN_ADDRESS} --ca-file ${ORDERER_CA} \
            --client-cert ${ORDERER_ADMIN_TLS_CERT} --client-key ${ORDERER_ADMIN_TLS_KEY}
        echo ""
        echo "[peer ${CORE_PEER_ADDRESS}]"
        peer channel getinfo -c ${CHANNEL_NAME} 2>&1 | grep -v "^$"
    ' 2>/dev/null
done
