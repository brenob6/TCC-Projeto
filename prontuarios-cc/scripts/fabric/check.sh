#!/usr/bin/env bash
#
# Executado em cada máquina: imprime, em uma linha, o estado do chaincode
# local (serviço, hash do binário, pacote aprovado, sequência confirmada e
# versão respondida pelo próprio chaincode).

source /vagrant/chaincode/common.sh

SERVICE=$(systemctl is-active prontuarios-cc 2>/dev/null)
BINARY=$(sha256sum /usr/local/bin/prontuarios-cc 2>/dev/null | cut -c1-16)
SEQUENCE=$(committed_sequence)
APPROVED=$(approved_package ${SEQUENCE:-1})
[ "${APPROVED}" = "${PACKAGE_ID}" ] && APPROVED=ok || APPROVED="divergente(${APPROVED:-nenhum})"
VERSION=$(peer chaincode query -C ${CHANNEL_NAME} -n ${CC_NAME} -c '{"Args":["getHeader"]}' 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin).get("version",""))' 2>/dev/null)

echo "$(hostname) servico=${SERVICE} binario=${BINARY} pacote=${APPROVED} sequencia=${SEQUENCE:-nenhuma} versao=${VERSION:-indisponivel}"
