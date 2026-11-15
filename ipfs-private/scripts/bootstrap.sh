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

# Cada nó mantém conexão (Peering.Peers) apenas com os nós de índice menor, de
# modo que cada par é conectado por um único lado. Conexões abertas pelos dois
# lados ao mesmo tempo são duplicadas e, ao fechar a excedente, o Bitswap deixa
# de enviar pedidos ao peer, embora a conexão restante continue ativa.
PEERING=""
for NODE in "${!IPS[@]}"
do
    if [ "${NODE##*-}" -lt "${HOSTNAME##*-}" ]; then
        PEER=$(cat ${SHARE}/peers/${NODE}.peer)
        IP=${IPS[$NODE]}

        echo "Adicionando $NODE"

        PEERING="${PEERING:+${PEERING},}{\"ID\":\"${PEER}\",\"Addrs\":[\"/ip4/${IP}/tcp/4001\"]}"
    fi
done

sudo -u ${IPFS_USER} ipfs config Peering.Peers "[${PEERING}]" --json

# Repetidos de provision.sh para as máquinas provisionadas antes da mudança.
sudo -u ${IPFS_USER} ipfs config Internal.Bitswap.BroadcastControl.Enable false --json
sudo -u ${IPFS_USER} ipfs config Discovery.MDNS.Enabled false --json

systemctl restart ipfs

# 'ipfs id' também responde com o daemon parado; 'swarm peers' exige o daemon.
until sudo -u ${IPFS_USER} ipfs swarm peers >/dev/null 2>&1; do
    sleep 1
done

sudo -u ${IPFS_USER} ipfs swarm peers
