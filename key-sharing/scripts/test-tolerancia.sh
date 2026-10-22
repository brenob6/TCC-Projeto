#!/usr/bin/env bash
#
# Valida o comportamento do esquema de limiar diante da indisponibilidade de
# custodiantes. Executado no host, pois utiliza o comando vagrant.
#
# Pré-requisito: os fluxos de scripts/test.sh já executados, de modo que a
# chave paciente-001 e o prontuário cifrado existam na máquina kms.

set -e

cd "$(dirname "$0")/.."

KEY_ID=${KEY_ID:-paciente-001}
ENC=/home/vagrant/kms/prontuario.txt.enc

separador() {
    echo ""
    echo "--------------------------------------"
    echo "$1"
    echo "--------------------------------------"
}

custodiante() {
    vagrant ssh "$1" -c "sudo systemctl $2 custodian" >/dev/null 2>&1
}

restaurar() {
    echo ""
    echo "Restaurando os custodiantes..."
    custodiante custodian-2 start || true
    custodiante custodian-3 start || true
}
trap restaurar EXIT

separador "1. Todos os custodiantes ativos (3 de 3)"
custodiante custodian-2 start || true
custodiante custodian-3 start || true
vagrant ssh kms -c "kms health"

separador "2. Um custodiante indisponível (2 de 3, limiar k=2)"
echo "Parando o custodian-3..."
custodiante custodian-3 stop

if ! vagrant ssh kms -c "cd /home/vagrant/kms && rm -f decifrado_prontuario.txt && \
        kms -requester medico-001 -institution hospital-a decrypt ${KEY_ID} ${ENC} && \
        diff -q prontuario.txt decifrado_prontuario.txt"; then
    echo "ERRO: a recuperação com 2 de 3 custodiantes deveria funcionar."
    exit 1
fi
echo "OK: com k=2, a queda de um custodiante não impede a recuperação."

separador "3. Dois custodiantes indisponíveis (1 de 3, abaixo do limiar)"
echo "Parando o custodian-2..."
custodiante custodian-2 stop

if vagrant ssh kms -c "cd /home/vagrant/kms && \
        kms -requester medico-001 -institution hospital-a decrypt ${KEY_ID} ${ENC}" \
        >/dev/null 2>&1; then
    echo "ERRO: a recuperação deveria falhar com apenas 1 custodiante ativo."
    exit 1
fi
echo "OK: abaixo do limiar, o prontuário permanece inacessível."

separador "4. Registro de auditoria em cada custodiante"
for node in custodian-1 custodian-2 custodian-3; do
    echo ""
    echo "[${node}]"
    custodiante "${node}" start || true
    vagrant ssh "${node}" -c \
        "sudo sqlite3 -header -column /var/lib/custodian/shares.db \
         'SELECT key_id, requester_id, institution_id, emergency, occurred_at FROM access_log'" \
        2>/dev/null || echo "(instale o sqlite3 na máquina para inspecionar a auditoria)"
done

separador "Testes de tolerância concluídos"
