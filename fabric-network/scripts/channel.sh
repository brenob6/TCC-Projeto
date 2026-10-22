#!/usr/bin/env bash
#
# Ingressa o orderer e o peer desta máquina no canal, a partir do bloco gênese
# gerado pelo configtxgen. Executado como provisionador nomeado (`channel`),
# após o provisionamento das três máquinas.
#
# Apenas configura o canal: nenhum chaincode é instalado e nenhuma transação
# é submetida.

set -e

source /etc/profile.d/fabric.sh

BLOCK=/vagrant/channel-artifacts/${CHANNEL_NAME}.block

osn() {
    osnadmin channel "$@" \
        -o ${ORDERER_ADMIN_ADDRESS} \
        --ca-file ${ORDERER_CA} \
        --client-cert ${ORDERER_ADMIN_TLS_CERT} \
        --client-key ${ORDERER_ADMIN_TLS_KEY}
}

# O osnadmin encerra com código 0 mesmo em respostas de erro; a participação
# é verificada pelo status HTTP (404 quando o canal não existe no orderer).
if osn list --channelID ${CHANNEL_NAME} 2>/dev/null | grep -q "Status: 200"; then
    echo "${ORDERER_ADDRESS} já participa do canal ${CHANNEL_NAME}."
else
    echo "Ingressando ${ORDERER_ADDRESS} no canal ${CHANNEL_NAME}..."
    osn join --channelID ${CHANNEL_NAME} --config-block ${BLOCK}
fi

if peer channel list 2>/dev/null | grep -qx "${CHANNEL_NAME}"; then
    echo "${CORE_PEER_ADDRESS} já participa do canal ${CHANNEL_NAME}."
else
    echo "Ingressando ${CORE_PEER_ADDRESS} no canal ${CHANNEL_NAME}..."
    peer channel join -b ${BLOCK}
fi

echo ""
echo "======================================"
echo "Canais do orderer ${ORDERER_ADDRESS}:"
osn list
echo ""
echo "Canais do peer ${CORE_PEER_ADDRESS}:"
peer channel list
echo "======================================"
