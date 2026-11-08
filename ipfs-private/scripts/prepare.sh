#!/usr/bin/env bash
#
# Executado no host antes de subir as máquinas de ../fabric-network, que
# também executam os nós IPFS: baixa o Kubo (uma única vez) e gera a
# swarm.key da rede privada, caso ainda não exista.

set -e

cd "$(dirname "$0")/.."

if [ ! -f downloads/kubo.tar.gz ]; then
    sh download-ipfs.sh
fi

if [ ! -f swarm.key ]; then
    echo "Gerando swarm.key da rede privada..."
    printf '/key/swarm/psk/1.0.0/\n/base16/\n%s\n' \
        "$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')" > swarm.key
    chmod 600 swarm.key
fi
