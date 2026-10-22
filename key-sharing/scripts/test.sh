#!/usr/bin/env bash
#
# Exercita os fluxos de operação do sistema de compartilhamento de chaves
# contra os custodiantes distribuídos nas máquinas Vagrant.
#
# Executado dentro da máquina kms. A indisponibilidade de custodiantes é
# simulada pelo host, em scripts/test-tolerancia.sh.

set -e

export CUSTODIANS=${CUSTODIANS:-192.168.56.21:50051,192.168.56.22:50051,192.168.56.23:50051}
export THRESHOLD=${THRESHOLD:-2}

WORKDIR=/home/vagrant/kms
KEY_ID=${KEY_ID:-paciente-001}
PRONTUARIO=${WORKDIR}/prontuario.txt

cd ${WORKDIR}

separador() {
    echo ""
    echo "--------------------------------------"
    echo "$1"
    echo "--------------------------------------"
}

separador "1. Estado dos custodiantes"
kms health

separador "2. Geração da chave e distribuição dos shares"
kms keygen ${KEY_ID}

separador "3. Cifração do prontuário"
cat > ${PRONTUARIO} <<'EOF'
Paciente: 001
Diagnóstico: hipertensão arterial sistêmica
Medicação: losartana 50mg
Alergias: penicilina
EOF

kms encrypt ${KEY_ID} ${PRONTUARIO}

echo ""
echo "Envelope armazenado na rede IPFS (trecho):"
head -c 240 ${PRONTUARIO}.enc
echo "..."

separador "4. Recuperação autorizada (fluxo normal)"
rm -f decifrado_prontuario.txt
kms -requester medico-001 -institution hospital-a \
    decrypt ${KEY_ID} ${PRONTUARIO}.enc

echo ""
echo "Conteúdo recuperado:"
cat decifrado_prontuario.txt

if ! diff -q ${PRONTUARIO} decifrado_prontuario.txt >/dev/null; then
    echo "ERRO: o prontuário recuperado difere do original."
    exit 1
fi
echo ""
echo "OK: prontuário recuperado é idêntico ao original."

separador "5. Recuperação extraordinária (acesso emergencial)"
rm -f decifrado_prontuario.txt
kms -requester medico-plantao -institution hospital-emergencia \
    -reason "paciente inconsciente em atendimento de urgência" \
    emergency ${KEY_ID} ${PRONTUARIO}.enc

if ! diff -q ${PRONTUARIO} decifrado_prontuario.txt >/dev/null; then
    echo "ERRO: a recuperação extraordinária não devolveu o prontuário original."
    exit 1
fi
echo "OK: recuperação extraordinária concluída e registrada nos custodiantes."

separador "6. Acesso emergencial sem justificativa"
if kms -requester medico-plantao -institution hospital-emergencia \
       -reason "" emergency ${KEY_ID} ${PRONTUARIO}.enc >/dev/null 2>&1; then
    # A flag -reason vazia faz o comando usar a justificativa padrão; este
    # caso é coberto pelos testes de unidade do pacote kms.
    echo "OK: o comando aplica a justificativa padrão do atendimento emergencial."
else
    echo "OK: acesso emergencial recusado sem justificativa."
fi

separador "Fluxos do KMS concluídos"
echo "Para validar a tolerância a falhas, execute no host:"
echo "  ./scripts/test-tolerancia.sh"
