#!/bin/bash
#
# Sobe o ambiente completo e executa os fluxos de operação.

set -e

cd "$(dirname "$0")/.."

./scripts/build.sh
./scripts/gen-tls.sh

# Os testes do módulo não dispõem da rede blockchain: os custodiantes são
# provisionados no modo isolado, sem a verificação no livro-razão. O
# ./start.sh da raiz os reprovisiona no modo livro-razao.
export CUSTODIAN_MODE=isolado
vagrant up --provision
vagrant provision kms --provision-with=test
