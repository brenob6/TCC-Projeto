#!/usr/bin/env bash
#
# Encerra o que foi iniciado por start.sh.
#
#   stop.sh          encerra a API Gateway; as máquinas continuam ligadas
#   stop.sh --vms    encerra também as máquinas virtuais (vagrant halt)
#
# Os dados (ledger, conteúdo IPFS, compartilhamentos e diretório da API) são
# preservados; ./start.sh retoma o ambiente.

set -e

ROOT=$(cd "$(dirname "$0")" && pwd)
RUN_DIR=${ROOT}/.run

HALT=0
case "$1" in
    "") ;;
    --vms) HALT=1 ;;
    -h|--help) sed -n '3,/^$/s/^# \{0,1\}//p' "$0"; exit 0 ;;
    *) echo "uso: $0 [--vms]" >&2; exit 2 ;;
esac

if [ -f ${RUN_DIR}/api.pid ] && kill -0 "$(cat ${RUN_DIR}/api.pid)" 2>/dev/null; then
    echo "Encerrando a API Gateway (PID $(cat ${RUN_DIR}/api.pid))..."
    kill "$(cat ${RUN_DIR}/api.pid)"
else
    echo "A API Gateway não está em execução."
fi
rm -f ${RUN_DIR}/api.pid

if [ ${HALT} -eq 1 ]; then
    # Desligar, em vez de suspender, evita a falha da rede host-only ao
    # retomar máquinas salvas. Os serviços sobem com as máquinas (systemd).
    for dir in fabric-network key-sharing; do
        echo ""
        echo "== ${dir} =="
        (cd ${ROOT}/${dir} && vagrant halt)
    done
fi
