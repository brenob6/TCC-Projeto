#!/usr/bin/env bash
#
# Provisiona um Custodiante de Chaves: instala o binário e registra o serviço.

set -e

CUSTODIAN_USER=vagrant
CUSTODIAN_ID=${1:-$(hostname)}
LISTEN_PORT=50051
DB_DIR=/var/lib/custodian

if [ ! -f /vagrant/build/custodian ]; then
    echo "ERRO: binário não encontrado em build/custodian."
    echo "Execute ./scripts/build.sh no host antes de subir as máquinas."
    exit 1
fi

echo "Instalando o custodiante ${CUSTODIAN_ID}..."
install -m 0755 /vagrant/build/custodian /usr/local/bin/custodian

# sqlite3 permite inspecionar a base local e o registro de auditoria.
if ! command -v sqlite3 >/dev/null 2>&1; then
    apt-get update -y >/dev/null
    apt-get install -y sqlite3 >/dev/null
fi

install -d -o ${CUSTODIAN_USER} -g ${CUSTODIAN_USER} -m 0700 ${DB_DIR}

cat > /etc/systemd/system/custodian.service <<EOF
[Unit]
Description=Custodiante de Chaves (gRPC)
After=network.target

[Service]
User=${CUSTODIAN_USER}
ExecStart=/usr/local/bin/custodian \\
    -id ${CUSTODIAN_ID} \\
    -listen 0.0.0.0:${LISTEN_PORT} \\
    -db ${DB_DIR}/shares.db
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable custodian
systemctl restart custodian

echo "Aguardando o custodiante aceitar conexões..."

for _ in $(seq 1 30); do
    if ss -ltn | grep -q ":${LISTEN_PORT}"; then
        break
    fi
    sleep 1
done

if ! ss -ltn | grep -q ":${LISTEN_PORT}"; then
    echo "ERRO: o custodiante não subiu. Últimas mensagens do serviço:"
    journalctl -u custodian -n 20 --no-pager
    exit 1
fi

echo ""
echo "======================================"
echo "Custodiante ${CUSTODIAN_ID} ativo na porta ${LISTEN_PORT}."
echo "Base local: ${DB_DIR}/shares.db"
echo "======================================"
