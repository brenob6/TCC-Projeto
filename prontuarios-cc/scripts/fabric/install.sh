#!/usr/bin/env bash
#
# Executado em cada máquina: instala o pacote CCaaS no peer e aprova a
# definição do chaincode para a organização local. Não altera o serviço do
# chaincode (ver service.sh).

set -e

source /vagrant/chaincode/common.sh

if peer lifecycle chaincode queryinstalled 2>/dev/null | grep -q "${PACKAGE_ID}"; then
    echo "Pacote ${PACKAGE_ID} já instalado."
else
    peer lifecycle chaincode install ${CC_DIR}/prontuarios-cc.tar.gz
fi

# Sem definição confirmada, aprova a sequência 1. Com a definição já
# confirmada, a organização pode reaprovar a mesma sequência apenas para
# apontar o peer local para um novo pacote.
SEQUENCE=$(committed_sequence)
SEQUENCE=${SEQUENCE:-1}
if [ "$(approved_package ${SEQUENCE})" = "${PACKAGE_ID}" ]; then
    echo "${CORE_PEER_LOCALMSPID} já aprovou ${PACKAGE_ID} na sequência ${SEQUENCE}."
else
    peer lifecycle chaincode approveformyorg -o ${ORDERER_ADDRESS} --tls --cafile ${ORDERER_CA} \
        -C ${CHANNEL_NAME} -n ${CC_NAME} -v ${CC_VERSION} --sequence ${SEQUENCE} \
        --package-id ${PACKAGE_ID} --waitForEvent
fi
