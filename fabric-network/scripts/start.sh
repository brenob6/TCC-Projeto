#!/bin/bash
#
# Sobe a rede completa: gera os artefatos, cria as VMs (orderer + peer em cada
# uma) e cria o canal, ingressando orderers e peers.

set -e

cd "$(dirname "$0")/.."

if [ ! -f downloads/fabric.tar.gz ]; then
    ./download-fabric.sh
fi

./scripts/generate.sh

vagrant up
vagrant provision --provision-with=channel
