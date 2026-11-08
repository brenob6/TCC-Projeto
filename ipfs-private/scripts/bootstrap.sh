#!/usr/bin/env bash

set -e

IPFS_USER=vagrant

# Diretório ipfs-private do host, montado pelo Vagrantfile de ../fabric-network.
SHARE=/ipfs-private

HOSTNAME=$(hostname)

# Os IPs repetem os definidos no Vagrantfile de ../fabric-network.
declare -A IPS=(
    ["fabric-node-1"]="192.168.56.31"
    ["fabric-node-2"]="192.168.56.32"
    ["fabric-node-3"]="192.168.56.33"
)

echo "Aguardando todos os Peer IDs..."

for NODE in "${!IPS[@]}"; do
    while [ ! -s ${SHARE}/peers/${NODE}.peer ]; do
        sleep 2
    done
done

echo "Removendo bootstraps..."
sudo -u ${IPFS_USER} ipfs bootstrap rm --all >/dev/null 2>&1 || true

for NODE in "${!IPS[@]}"
do
    if [ "$NODE" != "$HOSTNAME" ]; then
        PEER=$(cat ${SHARE}/peers/${NODE}.peer)
        IP=${IPS[$NODE]}

        echo "Adicionando $NODE"

        sudo -u ${IPFS_USER} ipfs bootstrap add \
            /ip4/$IP/tcp/4001/p2p/$PEER
    fi
done

sudo -u ${IPFS_USER} ipfs swarm peers
