#!/usr/bin/env bash
#
# Primeira implantação do chaincode na rede de ../../fabric-network (B-01):
# instala o pacote e o serviço do chaincode nas três máquinas, aprova a
# definição por organização e a confirma no canal prontuarios.
#
# Para alterações posteriores no código, use update.sh.

set -e

source "$(dirname "$0")/lib.sh"

prepare_machines
stage

echo ""
echo "== Instalação e aprovação =="
on_all "bash /vagrant/chaincode/install.sh"

echo ""
echo "== Serviço do chaincode =="
on_all "bash /vagrant/chaincode/service.sh prepare"
on_all_parallel "bash /vagrant/chaincode/service.sh activate"

echo ""
echo "== Confirmação da definição no canal =="
vm fabric-node-1 "bash /vagrant/chaincode/commit.sh"

echo ""
verify
