#!/usr/bin/env bash
#
# Provisiona uma máquina da rede Fabric: instala os binários e registra os
# serviços systemd do orderer (ordererN.example.com) e do peer
# (peer0.orgN.example.com), em que N é o índice recebido como argumento.

set -e

INDEX=${1:?informe o índice do nó (1, 2 ou 3)}

FABRIC_USER=vagrant
FABRIC_CFG=/etc/hyperledger/fabric
DATA_DIR=/var/hyperledger/production

ORG_DOMAIN=org${INDEX}.example.com
PEER_HOST=peer0.${ORG_DOMAIN}
ORDERER_HOST=orderer${INDEX}.example.com
MSP_ID=Org${INDEX}MSP

PEER_DIR=${FABRIC_CFG}/organizations/peerOrganizations/${ORG_DOMAIN}/peers/${PEER_HOST}
PEER_ADMIN_DIR=${FABRIC_CFG}/organizations/peerOrganizations/${ORG_DOMAIN}/users/Admin@${ORG_DOMAIN}
ORDERER_DIR=${FABRIC_CFG}/organizations/ordererOrganizations/example.com/orderers/${ORDERER_HOST}
ORDERER_ADMIN_DIR=${FABRIC_CFG}/organizations/ordererOrganizations/example.com/users/Admin@example.com

for f in /vagrant/downloads/fabric.tar.gz /vagrant/channel-artifacts/prontuarios.block; do
    if [ ! -f "$f" ]; then
        echo "ERRO: ${f#/vagrant/} não encontrado."
        echo "Execute ./download-fabric.sh e ./scripts/generate.sh no host antes de subir as máquinas."
        exit 1
    fi
done

# Os nomes dos nós precisam resolver para os IPs da rede privada, pois os
# certificados TLS e a configuração do canal referenciam os hostnames.
echo "Configurando /etc/hosts..."
sed -i '/example\.com/d' /etc/hosts
cat >> /etc/hosts <<HOSTS
192.168.56.31 orderer1.example.com peer0.org1.example.com
192.168.56.32 orderer2.example.com peer0.org2.example.com
192.168.56.33 orderer3.example.com peer0.org3.example.com
HOSTS

if ! command -v peer >/dev/null 2>&1; then
    echo "Instalando os binários do Fabric..."
    TMP=$(mktemp -d)
    tar -xzf /vagrant/downloads/fabric.tar.gz -C ${TMP}
    install -m 0755 ${TMP}/bin/* /usr/local/bin/

    # Builder externo para chaincodes como serviço (ccaas), já referenciado no
    # core.yaml padrão. Dispensa o Docker nas máquinas.
    install -d /opt/hyperledger/ccaas_builder
    cp -r ${TMP}/builders/ccaas/bin /opt/hyperledger/ccaas_builder/

    install -d ${FABRIC_CFG}
    cp ${TMP}/config/core.yaml ${TMP}/config/orderer.yaml ${FABRIC_CFG}/
    rm -rf ${TMP}
fi

# Sem Docker: chaincodes serão executados externamente (ccaas).
sed -i 's|endpoint: unix:///var/run/docker.sock|endpoint: ""|' ${FABRIC_CFG}/core.yaml

# Copia apenas o material criptográfico usado por esta máquina.
echo "Copiando o material criptográfico de ${MSP_ID} e ${ORDERER_HOST}..."
rm -rf ${FABRIC_CFG}/organizations
(
    cd /vagrant
    cp -r --parents \
        organizations/peerOrganizations/${ORG_DOMAIN}/peers/${PEER_HOST} \
        organizations/peerOrganizations/${ORG_DOMAIN}/users/Admin@${ORG_DOMAIN} \
        organizations/ordererOrganizations/example.com/orderers/${ORDERER_HOST} \
        organizations/ordererOrganizations/example.com/users/Admin@example.com \
        organizations/ordererOrganizations/example.com/tlsca \
        ${FABRIC_CFG}/
)
chown -R ${FABRIC_USER}:${FABRIC_USER} ${FABRIC_CFG}/organizations
chmod -R go-rwx ${FABRIC_CFG}/organizations

install -d -o ${FABRIC_USER} -g ${FABRIC_USER} ${DATA_DIR} ${DATA_DIR}/orderer ${DATA_DIR}/peer

cat > ${FABRIC_CFG}/orderer.env <<ENV
FABRIC_CFG_PATH=${FABRIC_CFG}
FABRIC_LOGGING_SPEC=INFO
ORDERER_GENERAL_LISTENADDRESS=0.0.0.0
ORDERER_GENERAL_LISTENPORT=7050
ORDERER_GENERAL_LOCALMSPID=OrdererMSP
ORDERER_GENERAL_LOCALMSPDIR=${ORDERER_DIR}/msp
ORDERER_GENERAL_BOOTSTRAPMETHOD=none
ORDERER_GENERAL_TLS_ENABLED=true
ORDERER_GENERAL_TLS_PRIVATEKEY=${ORDERER_DIR}/tls/server.key
ORDERER_GENERAL_TLS_CERTIFICATE=${ORDERER_DIR}/tls/server.crt
ORDERER_GENERAL_TLS_ROOTCAS=[${ORDERER_DIR}/tls/ca.crt]
ORDERER_GENERAL_CLUSTER_CLIENTCERTIFICATE=${ORDERER_DIR}/tls/server.crt
ORDERER_GENERAL_CLUSTER_CLIENTPRIVATEKEY=${ORDERER_DIR}/tls/server.key
ORDERER_GENERAL_CLUSTER_ROOTCAS=[${ORDERER_DIR}/tls/ca.crt]
ORDERER_CHANNELPARTICIPATION_ENABLED=true
ORDERER_ADMIN_LISTENADDRESS=0.0.0.0:7053
ORDERER_ADMIN_TLS_ENABLED=true
ORDERER_ADMIN_TLS_CERTIFICATE=${ORDERER_DIR}/tls/server.crt
ORDERER_ADMIN_TLS_PRIVATEKEY=${ORDERER_DIR}/tls/server.key
ORDERER_ADMIN_TLS_ROOTCAS=[${ORDERER_DIR}/tls/ca.crt]
ORDERER_ADMIN_TLS_CLIENTAUTHREQUIRED=true
ORDERER_ADMIN_TLS_CLIENTROOTCAS=[${ORDERER_DIR}/tls/ca.crt]
ORDERER_FILELEDGER_LOCATION=${DATA_DIR}/orderer
ORDERER_CONSENSUS_WALDIR=${DATA_DIR}/orderer/etcdraft/wal
ORDERER_CONSENSUS_SNAPDIR=${DATA_DIR}/orderer/etcdraft/snapshot
ORDERER_OPERATIONS_LISTENADDRESS=0.0.0.0:9443
ORDERER_METRICS_PROVIDER=prometheus
ENV

cat > ${FABRIC_CFG}/peer.env <<ENV
FABRIC_CFG_PATH=${FABRIC_CFG}
FABRIC_LOGGING_SPEC=INFO
CORE_PEER_ID=${PEER_HOST}
CORE_PEER_ADDRESS=${PEER_HOST}:7051
CORE_PEER_LISTENADDRESS=0.0.0.0:7051
CORE_PEER_CHAINCODEADDRESS=${PEER_HOST}:7052
CORE_PEER_CHAINCODELISTENADDRESS=0.0.0.0:7052
CORE_PEER_GOSSIP_BOOTSTRAP=${PEER_HOST}:7051
CORE_PEER_GOSSIP_EXTERNALENDPOINT=${PEER_HOST}:7051
CORE_PEER_LOCALMSPID=${MSP_ID}
CORE_PEER_MSPCONFIGPATH=${PEER_DIR}/msp
CORE_PEER_TLS_ENABLED=true
CORE_PEER_TLS_CERT_FILE=${PEER_DIR}/tls/server.crt
CORE_PEER_TLS_KEY_FILE=${PEER_DIR}/tls/server.key
CORE_PEER_TLS_ROOTCERT_FILE=${PEER_DIR}/tls/ca.crt
CORE_PEER_FILESYSTEMPATH=${DATA_DIR}/peer
CORE_LEDGER_SNAPSHOTS_ROOTDIR=${DATA_DIR}/peer/snapshots
CORE_OPERATIONS_LISTENADDRESS=0.0.0.0:9444
CORE_METRICS_PROVIDER=prometheus
ENV

for SERVICE in orderer peer; do
    if [ "${SERVICE}" = "peer" ]; then
        EXEC="/usr/local/bin/peer node start"
        DESC="Hyperledger Fabric peer (${PEER_HOST})"
    else
        EXEC="/usr/local/bin/orderer"
        DESC="Hyperledger Fabric orderer (${ORDERER_HOST})"
    fi

    cat > /etc/systemd/system/fabric-${SERVICE}.service <<UNIT
[Unit]
Description=${DESC}
After=network-online.target
Wants=network-online.target

[Service]
User=${FABRIC_USER}
EnvironmentFile=${FABRIC_CFG}/${SERVICE}.env
ExecStart=${EXEC}
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT
done

# Variáveis para operar a rede como administrador da organização em qualquer
# sessão da máquina (peer CLI e osnadmin).
cat > /etc/profile.d/fabric.sh <<PROFILE
export FABRIC_CFG_PATH=${FABRIC_CFG}
export CHANNEL_NAME=prontuarios
export CORE_PEER_LOCALMSPID=${MSP_ID}
export CORE_PEER_ADDRESS=${PEER_HOST}:7051
export CORE_PEER_MSPCONFIGPATH=${PEER_ADMIN_DIR}/msp
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_TLS_ROOTCERT_FILE=${PEER_DIR}/tls/ca.crt
export ORDERER_ADDRESS=${ORDERER_HOST}:7050
export ORDERER_ADMIN_ADDRESS=${ORDERER_HOST}:7053
export ORDERER_CA=${FABRIC_CFG}/organizations/ordererOrganizations/example.com/tlsca/tlsca.example.com-cert.pem
export ORDERER_ADMIN_TLS_CERT=${ORDERER_ADMIN_DIR}/tls/client.crt
export ORDERER_ADMIN_TLS_KEY=${ORDERER_ADMIN_DIR}/tls/client.key
PROFILE

systemctl daemon-reload
systemctl enable fabric-orderer fabric-peer
systemctl restart fabric-orderer fabric-peer

echo "Aguardando orderer e peer aceitarem conexões..."

for PORT in 7050 7053 7051; do
    for _ in $(seq 1 30); do
        ss -ltn | grep -q ":${PORT} " && break
        sleep 1
    done
    if ! ss -ltn | grep -q ":${PORT} "; then
        echo "ERRO: nenhum serviço escutando na porta ${PORT}. Últimas mensagens:"
        journalctl -u fabric-orderer -u fabric-peer -n 30 --no-pager
        exit 1
    fi
done

echo ""
echo "======================================"
echo "Fabric instalado em $(hostname)."
echo "  Orderer: ${ORDERER_HOST}:7050 (admin 7053)"
echo "  Peer:    ${PEER_HOST}:7051 (${MSP_ID})"
echo ""
echo "Crie o canal com: vagrant provision --provision-with=channel"
echo "======================================"
