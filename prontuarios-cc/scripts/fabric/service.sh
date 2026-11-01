#!/usr/bin/env bash
#
# Executado em cada máquina: gerencia o serviço systemd que executa o
# chaincode como serviço externo (CCaaS).
#
#   service.sh prepare    copia o novo binário e a configuração, sem reiniciar
#   service.sh activate   substitui o binário em uso e reinicia o serviço
#
# A separação permite preparar as três máquinas e depois ativá-las quase ao
# mesmo tempo, reduzindo o intervalo em que os peers executam versões
# diferentes do chaincode.

set -e

source /vagrant/chaincode/common.sh

BIN=/usr/local/bin/prontuarios-cc
ENV_FILE=/etc/prontuarios-cc/chaincode.env

prepare() {
    sudo install -m 0755 ${CC_DIR}/prontuarios-cc ${BIN}.new
    sudo install -m 0755 ${CC_DIR}/patient-signer /usr/local/bin/patient-signer
    sudo install -d -m 0750 /etc/prontuarios-cc
    sudo tee ${ENV_FILE}.new >/dev/null <<ENV
RUN_CCAAS=true
CHAINCODE_SERVER_ADDRESS=127.0.0.1:9999
CHAINCODE_ID=${PACKAGE_ID}
CPF_HASH_PEPPER=$(cat ${CC_DIR}/pepper)
ENV
    sudo chmod 600 ${ENV_FILE}.new

    sudo tee /etc/systemd/system/prontuarios-cc.service >/dev/null <<UNIT
[Unit]
Description=Chaincode prontuarios-cc (CCaaS)
After=network-online.target fabric-peer.service

[Service]
EnvironmentFile=${ENV_FILE}
ExecStart=${BIN}
Restart=always
RestartSec=5
DynamicUser=yes

[Install]
WantedBy=multi-user.target
UNIT
    sudo systemctl daemon-reload
    sudo systemctl enable prontuarios-cc >/dev/null 2>&1
    echo "$(hostname): binário e configuração preparados."
}

activate() {
    if [ ! -f ${BIN}.new ]; then
        echo "$(hostname): nada preparado; execute 'service.sh prepare' antes." >&2
        exit 1
    fi
    sudo mv ${BIN}.new ${BIN}
    sudo mv ${ENV_FILE}.new ${ENV_FILE}
    sudo systemctl restart prontuarios-cc
    sleep 1
    if ! systemctl is-active --quiet prontuarios-cc; then
        echo "$(hostname): o serviço não iniciou. Últimas mensagens:" >&2
        sudo journalctl -u prontuarios-cc -n 20 --no-pager >&2
        exit 1
    fi
    echo "$(hostname): serviço reiniciado."
}

case "$1" in
    prepare) prepare ;;
    activate) activate ;;
    *) echo "uso: service.sh prepare|activate" >&2; exit 2 ;;
esac
