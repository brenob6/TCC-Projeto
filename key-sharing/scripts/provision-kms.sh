#!/usr/bin/env bash
#
# Provisiona a máquina do KMS, que coordena os custodiantes.

set -e

KMS_USER=vagrant
WORKDIR=/home/${KMS_USER}/kms

if [ ! -f /vagrant/build/kms ]; then
    echo "ERRO: binário não encontrado em build/kms."
    echo "Execute ./scripts/build.sh no host antes de subir as máquinas."
    exit 1
fi

echo "Instalando o KMS..."
install -m 0755 /vagrant/build/kms /usr/local/bin/kms

install -d -o ${KMS_USER} -g ${KMS_USER} ${WORKDIR}

# Endereços dos custodiantes, disponíveis a qualquer sessão da máquina.
cat > /etc/profile.d/kms.sh <<'EOF'
export CUSTODIANS=192.168.56.21:50051,192.168.56.22:50051,192.168.56.23:50051
export THRESHOLD=2
EOF

echo ""
echo "======================================"
echo "KMS instalado."
echo "Esquema 2-de-3 sobre os custodiantes:"
echo "  192.168.56.21:50051"
echo "  192.168.56.22:50051"
echo "  192.168.56.23:50051"
echo ""
echo "Execute os testes com: vagrant provision kms --provision-with=test"
echo "======================================"
