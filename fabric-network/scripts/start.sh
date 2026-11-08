#!/bin/bash
#
# Sobe a rede completa: gera os artefatos, cria as VMs (orderer, peer e nó
# IPFS em cada uma), cria o canal, ingressando orderers e peers, e interliga
# os nós da rede IPFS privada.

set -e

cd "$(dirname "$0")/.."

if [ ! -f downloads/fabric.tar.gz ]; then
    ./download-fabric.sh
fi

../ipfs-private/scripts/prepare.sh
./scripts/generate.sh

vagrant up
vagrant provision --provision-with=channel
vagrant provision --provision-with=ipfs-bootstrap
