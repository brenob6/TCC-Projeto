#!/usr/bin/env bash

set -e

IPFS_USER=vagrant

HOSTNAME=$(hostname)

declare -A IPS=(
    ["ipfs-node-1"]="192.168.56.11"
    ["ipfs-node-2"]="192.168.56.12"
    ["ipfs-node-3"]="192.168.56.13"
)

echo "Aguardando todos os Peer IDs..."

while [ "$(find /vagrant/peers -name '*.peer' | wc -l)" -lt 3 ]; do
    sleep 2
done

echo "Removendo bootstraps..."
sudo -u ${IPFS_USER} ipfs bootstrap rm --all >/dev/null 2>&1 || true

for FILE in /vagrant/peers/*.peer
do
    NODE=$(basename "$FILE" .peer)

    if [ "$NODE" != "$HOSTNAME" ]; then
        PEER=$(cat "$FILE")
        IP=${IPS[$NODE]}

        echo "Adicionando $NODE"

        sudo -u ${IPFS_USER} ipfs bootstrap add \
            /ip4/$IP/tcp/4001/p2p/$PEER
    fi
done

sudo -u ${IPFS_USER} ipfs swarm peers
