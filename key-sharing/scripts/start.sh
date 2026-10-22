#!/bin/bash
#
# Sobe o ambiente completo e executa os fluxos de operação.

set -e

cd "$(dirname "$0")/.."

./scripts/build.sh

vagrant up
vagrant provision kms --provision-with=test
