# Variáveis e funções comuns aos scripts executados nas máquinas da rede
# Fabric (copiados para /vagrant/chaincode pelos scripts de ../vagrant).

source /etc/profile.d/fabric.sh

CC_NAME=prontuarios
CC_VERSION=1.0
CC_DIR=/vagrant/chaincode
PACKAGE_ID=$(cat ${CC_DIR}/package-id)

# Certificados TLS (públicos) dos peers das três organizações.
peer_tls_ca() {
    echo /vagrant/organizations/peerOrganizations/org$1.example.com/peers/peer0.org$1.example.com/tls/ca.crt
}

# Sequência confirmada no canal (vazio se a definição ainda não existe).
committed_sequence() {
    peer lifecycle chaincode querycommitted -C ${CHANNEL_NAME} -n ${CC_NAME} 2>/dev/null \
        | sed -n 's/.*Sequence: \([0-9]*\).*/\1/p'
}

# Package ID aprovado pela organização local para a sequência $1.
approved_package() {
    peer lifecycle chaincode queryapproved -C ${CHANNEL_NAME} -n ${CC_NAME} --sequence $1 2>/dev/null \
        | sed -n 's/.*package-id: \([^,]*\),.*/\1/p'
}
