# !/usr/bin/sh

set -e

IPFS_USER=vagrant

# Diretório ipfs-private do host, montado pelo Vagrantfile de ../fabric-network.
SHARE=/ipfs-private

echo "Updating system packages..."
sudo apt update -y


if ! command -v ipfs >/dev/null 2>&1; then
		echo "IPFS is not installed. Installing IPFS..."
		cp ${SHARE}/downloads/kubo.tar.gz .
		tar -xvzf kubo.tar.gz
		
		cd kubo
		sudo bash install.sh -y
		cd ..
fi

sudo -u ${IPFS_USER} bash <<EOF

export IPFS_PATH=/home/${IPFS_USER}/.ipfs

if [ ! -d "\$IPFS_PATH" ]; then
    ipfs init
fi

ipfs bootstrap rm --all

cp ${SHARE}/swarm.key \$IPFS_PATH/swarm.key

chmod 600 \$IPFS_PATH/swarm.key

ipfs config Addresses.API "/ip4/0.0.0.0/tcp/5001"
ipfs config Addresses.Gateway "/ip4/0.0.0.0/tcp/8080"
ipfs config AutoConf.Enabled false --json

EOF

cat > /etc/systemd/system/ipfs.service <<EOF
[Unit]
Description=IPFS daemon
After=network.target

[Service]
User=${IPFS_USER}
Environment=IPFS_PATH=/home/${IPFS_USER}/.ipfs
ExecStart=/usr/local/bin/ipfs daemon
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable ipfs
systemctl start ipfs

echo "Aguardando daemon iniciar..."

until sudo -u vagrant ipfs id >/dev/null 2>&1; do
    sleep 1
done

echo ""
echo "======================================"
echo "IPFS instalado e configurado."
echo "Peer ID:"
sudo -u ${IPFS_USER} ipfs id
echo "======================================"

mkdir -p ${SHARE}/peers

HOSTNAME=$(hostname)
PEER_ID=$(sudo -u ${IPFS_USER} ipfs id -f="<id>")

echo "$PEER_ID" > ${SHARE}/peers/${HOSTNAME}.peer
