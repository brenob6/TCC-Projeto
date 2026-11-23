#!/usr/bin/env bash
#
# Provisiona um Custodiante de Chaves: instala o binário, o material TLS e a
# conexão com o livro-razão, e registra o serviço.
#
# Uso: provision.sh <id> [livro-razao|isolado]
#
# No modo livro-razao (padrão), cada decifração parcial exige a confirmação da
# autorização no livro-razão, com a configuração gerada no host por
# scripts/ledger-config.sh. O modo isolado dispensa essa confirmação e existe
# apenas para os testes do módulo sem a rede blockchain (scripts/start.sh).

set -e

CUSTODIAN_USER=vagrant
CUSTODIAN_ID=${1:-$(hostname)}
MODE=${2:-livro-razao}
LISTEN_PORT=50051
DB_DIR=/var/lib/custodian
CONF_DIR=/etc/custodian
TLS_SRC=/vagrant/build/tls
LEDGER_SRC=/vagrant/build/ledger/${CUSTODIAN_ID}

if [ ! -f /vagrant/build/custodian ]; then
    echo "ERRO: binário não encontrado em build/custodian."
    echo "Execute ./scripts/build.sh no host antes de subir as máquinas."
    exit 1
fi

for f in ca.crt ${CUSTODIAN_ID}.crt ${CUSTODIAN_ID}.key; do
    if [ ! -f ${TLS_SRC}/${f} ]; then
        echo "ERRO: ${TLS_SRC}/${f} não encontrado."
        echo "Execute ./scripts/gen-tls.sh no host antes de subir as máquinas."
        exit 1
    fi
done

case ${MODE} in
    livro-razao)
        if [ ! -f ${LEDGER_SRC}/ledger.json ]; then
            echo "ERRO: ${LEDGER_SRC}/ledger.json não encontrado."
            echo "Execute ./scripts/ledger-config.sh <fabric-network/organizations> no host,"
            echo "ou use CUSTODIAN_MODE=isolado apenas para os testes do módulo."
            exit 1
        fi
        ;;
    isolado) ;;
    *) echo "ERRO: modo desconhecido: ${MODE}"; exit 1 ;;
esac

echo "Instalando o custodiante ${CUSTODIAN_ID} (modo ${MODE})..."
install -m 0755 /vagrant/build/custodian /usr/local/bin/custodian

# Somente o material do próprio custodiante é copiado para fora da pasta
# compartilhada, com acesso restrito ao usuário do serviço.
rm -rf ${CONF_DIR}
install -d -o ${CUSTODIAN_USER} -g ${CUSTODIAN_USER} -m 0700 ${CONF_DIR} ${CONF_DIR}/tls
install -o ${CUSTODIAN_USER} -g ${CUSTODIAN_USER} -m 0644 ${TLS_SRC}/ca.crt ${CONF_DIR}/tls/ca.crt
install -o ${CUSTODIAN_USER} -g ${CUSTODIAN_USER} -m 0644 ${TLS_SRC}/${CUSTODIAN_ID}.crt ${CONF_DIR}/tls/server.crt
install -o ${CUSTODIAN_USER} -g ${CUSTODIAN_USER} -m 0600 ${TLS_SRC}/${CUSTODIAN_ID}.key ${CONF_DIR}/tls/server.key

if [ ${MODE} = livro-razao ]; then
    install -d -o ${CUSTODIAN_USER} -g ${CUSTODIAN_USER} -m 0700 ${CONF_DIR}/ledger
    for f in ledger.json tls-ca.crt cert.pem; do
        install -o ${CUSTODIAN_USER} -g ${CUSTODIAN_USER} -m 0644 ${LEDGER_SRC}/${f} ${CONF_DIR}/ledger/${f}
    done
    install -o ${CUSTODIAN_USER} -g ${CUSTODIAN_USER} -m 0600 ${LEDGER_SRC}/key.pem ${CONF_DIR}/ledger/key.pem
    LEDGER_ARG="-ledger ${CONF_DIR}/ledger/ledger.json"
else
    LEDGER_ARG="-isolado"
fi

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
    -db ${DB_DIR}/shares.db \\
    -tls-ca ${CONF_DIR}/tls/ca.crt \\
    -tls-cert ${CONF_DIR}/tls/server.crt \\
    -tls-key ${CONF_DIR}/tls/server.key \\
    ${LEDGER_ARG}
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
echo "Custodiante ${CUSTODIAN_ID} ativo na porta ${LISTEN_PORT} (TLS mútuo, modo ${MODE})."
echo "Base local: ${DB_DIR}/shares.db"
echo "======================================"
