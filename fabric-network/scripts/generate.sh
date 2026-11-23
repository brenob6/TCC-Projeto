#!/usr/bin/env bash
#
# Gera, no host, o material criptográfico das organizações (cryptogen) e o
# bloco gênese do canal (configtxgen). Executado uma única vez, antes de
# `vagrant up`; os artefatos são lidos pelas VMs pela pasta compartilhada.
#
# Para regenerar do zero, remova organizations/ e channel-artifacts/ e
# recrie as VMs (vagrant destroy -f), pois os ledgers existentes deixam de
# ser compatíveis com os novos certificados.

set -e

cd "$(dirname "$0")/.."

CHANNEL_NAME=prontuarios
FABRIC_DIR=downloads/fabric

if [ ! -f downloads/fabric.tar.gz ]; then
    echo "ERRO: downloads/fabric.tar.gz não encontrado."
    echo "Execute ./download-fabric.sh antes."
    exit 1
fi

if [ ! -x ${FABRIC_DIR}/bin/cryptogen ]; then
    mkdir -p ${FABRIC_DIR}
    tar -xzf downloads/fabric.tar.gz -C ${FABRIC_DIR}
fi

export PATH=${PWD}/${FABRIC_DIR}/bin:$PATH

if [ -d organizations ]; then
    echo "organizations/ já existe; mantendo o material criptográfico atual."
    # Redes criadas antes das identidades dos custodiantes (User2) recebem
    # apenas o que falta: o cryptogen extend reutiliza as CAs existentes e não
    # altera os certificados já emitidos.
    if ls organizations/peerOrganizations/*/users | grep -q "^User1@" &&
       [ "$(ls -d organizations/peerOrganizations/*/users/User2@* 2>/dev/null | wc -l)" -lt 3 ]; then
        echo "Gerando as identidades cliente dos custodiantes (User2)..."
        cryptogen extend --config=config/crypto-config.yaml --input=organizations
    fi
else
    echo "Gerando o material criptográfico..."
    cryptogen generate --config=config/crypto-config.yaml --output=organizations
fi

if [ -f channel-artifacts/${CHANNEL_NAME}.block ]; then
    echo "channel-artifacts/${CHANNEL_NAME}.block já existe; mantendo o bloco atual."
else
    echo "Gerando o bloco gênese do canal ${CHANNEL_NAME}..."
    mkdir -p channel-artifacts
    configtxgen -configPath config \
        -profile ProntuariosChannel \
        -channelID ${CHANNEL_NAME} \
        -outputBlock channel-artifacts/${CHANNEL_NAME}.block
fi

echo ""
echo "Artefatos prontos:"
echo "  organizations/"
echo "  channel-artifacts/${CHANNEL_NAME}.block"
