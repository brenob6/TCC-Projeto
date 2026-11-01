#!/usr/bin/env bash
#
# Confirma a definição do chaincode no canal, coletando o endosso dos três
# peers (política LifecycleEndorsement: MAJORITY).

set -e

source /vagrant/chaincode/common.sh

SEQUENCE=1
if [ -n "$(committed_sequence)" ]; then
    echo "Definição já confirmada; nada a fazer."
else
    peer lifecycle chaincode commit -o ${ORDERER_ADDRESS} --tls --cafile ${ORDERER_CA} \
        -C ${CHANNEL_NAME} -n ${CC_NAME} -v ${CC_VERSION} --sequence ${SEQUENCE} \
        --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles $(peer_tls_ca 1) \
        --peerAddresses peer0.org2.example.com:7051 --tlsRootCertFiles $(peer_tls_ca 2) \
        --peerAddresses peer0.org3.example.com:7051 --tlsRootCertFiles $(peer_tls_ca 3) \
        --waitForEvent
fi

peer lifecycle chaincode querycommitted -C ${CHANNEL_NAME} -n ${CC_NAME}
