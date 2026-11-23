#!/usr/bin/env bash
#
# Atualiza o chaincode em uma rede em que ele já foi implantado (deploy.sh).
#
#   update.sh            recompila, distribui e reinicia o chaincode
#
# No modelo CCaaS o pacote registrado no canal contém apenas o endereço do
# serviço, não o código. Assim, uma mudança de código não exige nova
# aprovação: basta substituir o binário e reiniciar o serviço em cada máquina.
# Se o pacote mudar (rótulo ou porta), cada organização reaprova a sequência
# vigente apontando para o novo pacote.
#
# Mudanças na definição (política de endosso, coleções privadas, versão)
# exigem uma nova sequência e não são tratadas aqui.
#
# Os dados do ledger são preservados. Não altere build/pepper: os pacientes já
# cadastrados deixariam de ser localizados pelo CPF.

set -e

source "$(dirname "$0")/lib.sh"

case "$1" in
    "") ;;
    *) echo "uso: $0" >&2; exit 2 ;;
esac

echo "== Testes unitários =="
(cd ${ROOT}/chaincode && go test ./...)

echo ""
echo "== Máquinas e rede =="
prepare_machines
if ! vm fabric-node-1 'source /etc/profile.d/fabric.sh &&
    peer lifecycle chaincode querycommitted -C ${CHANNEL_NAME} -n prontuarios >/dev/null 2>&1'; then
    echo "ERRO: o chaincode ainda não foi implantado; execute deploy.sh." >&2
    exit 1
fi

stage

echo ""
echo "== Pacote e aprovação =="
on_all "bash /vagrant/chaincode/install.sh"

echo ""
echo "== Substituição do binário =="
# Prepara as três máquinas e só então reinicia todas ao mesmo tempo.
on_all "bash /vagrant/chaincode/service.sh prepare"
on_all_parallel "bash /vagrant/chaincode/service.sh activate"

echo ""
echo "== Verificação =="
verify
