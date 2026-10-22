#!/usr/bin/env bash
#
# Baixa os binários do Hyperledger Fabric (linux-amd64) para downloads/.
# O download é feito no host para que as VMs não precisem baixá-los
# individualmente; os mesmos binários são usados no host para gerar o
# material criptográfico e o bloco do canal.

set -e

cd "$(dirname "$0")"

FABRIC_VERSION="2.5.16"

mkdir -p downloads

wget https://github.com/hyperledger/fabric/releases/download/v${FABRIC_VERSION}/hyperledger-fabric-linux-amd64-${FABRIC_VERSION}.tar.gz \
    -O downloads/fabric.tar.gz
