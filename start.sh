#!/usr/bin/env bash
#
# Sobe todos os módulos da prova de conceito e deixa o frontend disponível
# para testes em http://127.0.0.1:8080:
#
#   1. custodiantes de chaves          (key-sharing, 3 máquinas)
#   2. instituições: rede Fabric, nós IPFS e chaincode
#                                      (fabric-network + ipfs-private + prontuarios-cc, 3 máquinas)
#   3. frontend compilado              (frontend/dist)
#   4. API Gateway em segundo plano    (api-gateway, servindo também o frontend)
#
# Na primeira execução, cria e provisiona as máquinas, gera os artefatos e
# implanta o chaincode. Nas seguintes, apenas liga ou retoma o que estiver
# parado e atualiza o que mudou. Pode ser executado novamente a qualquer
# momento; a API é reiniciada com o código atual.
#
# Para encerrar: ./stop.sh (somente a API) ou ./stop.sh --vms (também as
# máquinas virtuais).

set -e

ROOT=$(cd "$(dirname "$0")" && pwd)
RUN_DIR=${ROOT}/.run
API_LISTEN=${API_LISTEN:-127.0.0.1:8080}
API_URL=http://${API_LISTEN}

# Os IPs e a memória repetem os definidos nos Vagrantfiles de cada módulo. Cada
# máquina fabric-node-N executa o orderer, o peer e o nó IPFS da instituição.
CUSTODIANS=(custodian-1:192.168.56.21 custodian-2:192.168.56.22 custodian-3:192.168.56.23)
FABRIC_NODES=(fabric-node-1:192.168.56.31 fabric-node-2:192.168.56.32 fabric-node-3:192.168.56.33)
FABRIC_MEMORY=2048
CUSTODIAN_MEMORY=512

case "$1" in
    "") ;;
    -h|--help) sed -n '3,/^$/s/^# \{0,1\}//p' "$0"; exit 0 ;;
    *) echo "uso: $0 [-h|--help]" >&2; exit 2 ;;
esac

step() {
    echo ""
    echo "======================================================================"
    echo "== $*"
    echo "======================================================================"
}

fail() {
    echo "ERRO: $*" >&2
    exit 1
}

# machine_states <diretório> [máquinas...]: imprime "<máquina> <estado>" para
# cada máquina do ambiente Vagrant (running, saved, poweroff, not_created...).
machine_states() {
    local dir=$1
    shift
    (cd ${dir} && vagrant status --machine-readable "$@") \
        | awk -F, '$3 == "state" {print $2, $4}'
}

# managed_machines <estados>: imprime "<diretório> <máquina>" para cada máquina
# usada por este script em um dos estados informados (expressão regular).
managed_machines() {
    local state=$1
    {
        machine_states ${ROOT}/key-sharing custodian-1 custodian-2 custodian-3 | sed "s|^|${ROOT}/key-sharing |"
        machine_states ${ROOT}/fabric-network | sed "s|^|${ROOT}/fabric-network |"
    } | awk -v s="^(${state})$" '$3 ~ s {print $1, $2}'
}

# vm_info <id> <campo>: valor de um campo de 'VBoxManage showvminfo', como
# VMState (running, saved, poweroff...) ou memory (MB).
vm_info() {
    VBoxManage showvminfo $1 --machinereadable | sed -n "s/^$2=\"\{0,1\}\([^\"]*\)\"\{0,1\}$/\1/p"
}

# expected_memory <diretório>: memória, em MB, definida no Vagrantfile.
expected_memory() {
    case $1 in
        */fabric-network) echo ${FABRIC_MEMORY} ;;
        */key-sharing) echo ${CUSTODIAN_MEMORY} ;;
    esac
}

# any_not_created <diretório> [máquinas...]: verdadeiro se alguma máquina
# ainda não foi criada.
any_not_created() {
    machine_states "$@" | grep -q " not_created$"
}

# reachable <ip> <porta>: verdadeiro se a porta TCP aceita conexões do host.
reachable() {
    timeout 3 bash -c "</dev/tcp/$1/$2" 2>/dev/null
}

# ensure_reachable <diretório> <porta> <máquina:ip>...: confere se o host
# alcança o serviço de cada máquina. Após retomar máquinas de um estado salvo
# do VirtualBox, a rede host-only pode ficar inacessível; nesse caso, reativa
# o enlace do adaptador e limpa a tabela ARP da máquina, como em
# prontuarios-cc/scripts/vagrant/lib.sh.
ensure_reachable() {
    local dir=$1 port=$2
    shift 2
    local entry name ip id ok
    for entry in "$@"; do
        name=${entry%%:*}
        ip=${entry##*:}
        ok=0
        for _ in $(seq 1 20); do
            if reachable ${ip} ${port}; then
                ok=1
                break
            fi
            sleep 3
        done
        if [ ${ok} -eq 0 ]; then
            echo "${name}: ${ip}:${port} inacessível; reativando o enlace da rede privada..."
            id=$(cat ${dir}/.vagrant/machines/${name}/virtualbox/id)
            VBoxManage controlvm ${id} setlinkstate2 off
            VBoxManage controlvm ${id} setlinkstate2 on
            sleep 3
            (cd ${dir} && vagrant ssh ${name} -c "sudo ip neigh flush dev eth1" -- -q)
            for _ in $(seq 1 10); do
                if reachable ${ip} ${port}; then
                    ok=1
                    break
                fi
                sleep 3
            done
        fi
        [ ${ok} -eq 1 ] || fail "${name} (${ip}:${port}) continua inacessível a partir do host."
        echo "${name}: ${ip}:${port} acessível."
    done
}

# ------------------------------------------------------------------------------
step "Pré-requisitos"
# ------------------------------------------------------------------------------

for cmd in vagrant VBoxManage go npm wget curl; do
    command -v ${cmd} >/dev/null 2>&1 || fail "comando '${cmd}' não encontrado."
done
echo "vagrant, VirtualBox, Go, npm, wget e curl disponíveis."

# ------------------------------------------------------------------------------
step "Máquinas virtuais"
# ------------------------------------------------------------------------------

# Uma máquina em estado salvo (vagrant suspend) só pode ser retomada com a
# mesma porta SSH do host que usava ao ser suspensa, e o Vagrant não consegue
# corrigir colisões nesse estado (as máquinas dos três módulos compartilham as
# portas 2222 e 2200 em diante); no estado aborted-saved, nem consegue retomá-la.
# Por isso, se houver máquinas salvas, o ambiente é desligado de forma
# ordenada: as ligadas são desligadas e cada salva é restaurada pelo
# VirtualBox e desligada pelo botão de energia ACPI, sem depender do SSH.
# Depois, todas iniciam do zero e o Vagrant escolhe portas livres. Os serviços
# sobem com as máquinas (systemd) e os dados são preservados.
saved=$(managed_machines "saved|aborted-saved")
if [ -n "${saved}" ]; then
    echo "Máquinas em estado salvo:" $(echo "${saved}" | awk '{print $2}')
    echo "Desligando o ambiente de forma ordenada para reiniciá-lo do zero..."
    while read -r dir name; do
        (cd ${dir} && vagrant halt ${name}) </dev/null
    done < <(managed_machines running)
    while read -r dir name; do
        id=$(cat ${dir}/.vagrant/machines/${name}/virtualbox/id)
        echo "${name}: restaurando e desligando..."
        VBoxManage startvm ${id} --type headless >/dev/null
        VBoxManage controlvm ${id} acpipowerbutton
        for _ in $(seq 1 60); do
            [ "$(vm_info ${id} VMState)" = "poweroff" ] && break
            sleep 2
        done
        [ "$(vm_info ${id} VMState)" = "poweroff" ] \
            || fail "${name} não desligou após o sinal ACPI; verifique-a no VirtualBox."
    done <<< "${saved}"
fi

# A memória definida no Vagrantfile só é aplicada quando a máquina inicia.
# Máquinas ligadas com outro valor são desligadas e iniciam a seguir com o
# valor atual.
while read -r dir name; do
    id=$(cat ${dir}/.vagrant/machines/${name}/virtualbox/id)
    memory=$(vm_info ${id} memory)
    if [ "${memory}" != "$(expected_memory ${dir})" ]; then
        echo "${name}: ${memory} MB em uso, $(expected_memory ${dir}) MB no Vagrantfile; reiniciando a máquina."
        (cd ${dir} && vagrant halt ${name}) </dev/null
    fi
done < <(managed_machines running)

# Memória exigida pelas máquinas que ainda não estão em execução.
needed=0
while read -r dir _; do
    needed=$((needed + $(expected_memory ${dir})))
done < <(managed_machines "poweroff|aborted|not_created")
available=$(awk '/^MemAvailable:/ {print int($2 / 1024)}' /proc/meminfo)
echo "Memória: ${needed} MB necessários para as máquinas paradas, ${available} MB disponíveis."
if [ ${needed} -gt ${available} ]; then
    echo "AVISO: memória possivelmente insuficiente; encerre outras máquinas virtuais se houver falhas."
fi

# ------------------------------------------------------------------------------
step "1/4 Custodiantes de chaves (key-sharing)"
# ------------------------------------------------------------------------------

cd ${ROOT}/key-sharing

# A API Gateway coordena os custodiantes diretamente (kmsclient); a máquina
# kms, usada apenas nos testes do módulo, não é necessária.
./scripts/build.sh
# O provisionamento reinstala o binário recém-compilado e reinicia o serviço;
# a base de compartilhamentos é preservada.
vagrant up --provision custodian-1 custodian-2 custodian-3

ensure_reachable ${ROOT}/key-sharing 50051 "${CUSTODIANS[@]}"

# ------------------------------------------------------------------------------
step "2/4 Instituições: rede Fabric, nós IPFS e chaincode (fabric-network + ipfs-private + prontuarios-cc)"
# ------------------------------------------------------------------------------

cd ${ROOT}/fabric-network

# Kubo e swarm.key, usados pelo provisionador ipfs das máquinas.
../ipfs-private/scripts/prepare.sh

if any_not_created ${ROOT}/fabric-network; then
    ./scripts/start.sh
elif [ ! -d organizations ] || [ ! -f channel-artifacts/prontuarios.block ]; then
    fail "as máquinas Fabric existem, mas organizations/ ou channel-artifacts/ não.
Recrie a rede: cd fabric-network && vagrant destroy -f && rm -rf organizations channel-artifacts"
fi

# Subshell: lib.sh redefine ROOT (diretório prontuarios-cc) e outras variáveis.
CC_SCRIPTS=${ROOT}/prontuarios-cc/scripts
(
    source ${CC_SCRIPTS}/vagrant/lib.sh

    # Liga ou retoma as máquinas, repara a rede privada e aguarda o líder Raft.
    prepare_machines

    if ! vm fabric-node-1 'source /etc/profile.d/fabric.sh &&
        peer lifecycle chaincode querycommitted -C ${CHANNEL_NAME} -n prontuarios >/dev/null 2>&1'; then
        echo "Chaincode ainda não implantado; executando deploy.sh..."
        ${CC_SCRIPTS}/vagrant/deploy.sh
    else
        # A compilação é reprodutível: se o binário gerado for igual ao das
        # máquinas e os serviços estiverem ativos, nada precisa ser feito.
        ${CC_SCRIPTS}/build.sh >/dev/null
        if ! verify; then
            echo "Chaincode desatualizado ou parado; executando update.sh..."
            ${CC_SCRIPTS}/vagrant/update.sh
        fi
    fi
)

ensure_reachable ${ROOT}/fabric-network 7051 "${FABRIC_NODES[@]}"

# Máquinas criadas antes de o nó IPFS fazer parte delas recebem o provisionador
# ipfs agora; em seguida, os três nós são interligados.
missing=""
for entry in "${FABRIC_NODES[@]}"; do
    name=${entry%%:*}
    vagrant ssh ${name} -c "test -f /etc/systemd/system/ipfs.service" -- -q </dev/null \
        || missing="${missing} ${name}"
done
if [ -n "${missing}" ]; then
    echo "Instalando o nó IPFS em:${missing}"
    vagrant provision --provision-with=ipfs ${missing}
    vagrant provision --provision-with=ipfs-bootstrap
fi

ensure_reachable ${ROOT}/fabric-network 5001 "${FABRIC_NODES[@]}"

# Garante a malha entre os nós (necessária para que uma instituição leia um
# envelope adicionado pelo nó de outra) e confere a conexão de cada nó com os
# outros dois.
ipfs_api() {
    curl -fsS -m 10 -X POST "http://$1:5001/api/v0/$2"
}
declare -A PEER_IDS
for entry in "${FABRIC_NODES[@]}"; do
    ip=${entry##*:}
    PEER_IDS[${ip}]=$(ipfs_api ${ip} id | sed -n 's/.*"ID":"\([^"]*\)".*/\1/p')
    [ -n "${PEER_IDS[${ip}]}" ] || fail "não foi possível obter o Peer ID do IPFS em ${entry%%:*}."
done
for entry in "${FABRIC_NODES[@]}"; do
    ip=${entry##*:}
    connected=0
    for other in "${!PEER_IDS[@]}"; do
        [ "${other}" = "${ip}" ] && continue
        if ipfs_api ${ip} "swarm/connect?arg=/ip4/${other}/tcp/4001/p2p/${PEER_IDS[${other}]}" >/dev/null; then
            connected=$((connected + 1))
        fi
    done
    echo "${entry%%:*}: IPFS conectado a ${connected} de 2 nós."
    [ ${connected} -eq 2 ] || echo "AVISO: o nó IPFS de ${entry%%:*} não se conectou a todos os demais."
done

# ------------------------------------------------------------------------------
step "3/4 Frontend (frontend)"
# ------------------------------------------------------------------------------

cd ${ROOT}/frontend

if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then
    npm ci
fi
npm run build

# ------------------------------------------------------------------------------
step "4/4 API Gateway (api-gateway)"
# ------------------------------------------------------------------------------

cd ${ROOT}/api-gateway
mkdir -p ${RUN_DIR}

# Encerra uma instância iniciada anteriormente por este script.
if [ -f ${RUN_DIR}/api.pid ] && kill -0 "$(cat ${RUN_DIR}/api.pid)" 2>/dev/null; then
    echo "Encerrando a API em execução (PID $(cat ${RUN_DIR}/api.pid))..."
    kill "$(cat ${RUN_DIR}/api.pid)"
    for _ in $(seq 1 10); do
        kill -0 "$(cat ${RUN_DIR}/api.pid)" 2>/dev/null || break
        sleep 1
    done
fi
rm -f ${RUN_DIR}/api.pid

if reachable ${API_LISTEN%:*} ${API_LISTEN##*:}; then
    fail "o endereço ${API_LISTEN} já está em uso por outro processo.
Encerre-o ou defina outro endereço: API_LISTEN=127.0.0.1:8081 $0"
fi

go build -o build/api ./cmd/api

# Os caminhos padrão da API (../fabric-network/organizations e
# data/directory.json) são relativos a api-gateway/.
nohup ./build/api -listen ${API_LISTEN} -static ../frontend/dist >> ${RUN_DIR}/api.log 2>&1 &
echo $! > ${RUN_DIR}/api.pid
echo "API iniciada (PID $(cat ${RUN_DIR}/api.pid)); log em .run/api.log."

health=""
for _ in $(seq 1 30); do
    if ! kill -0 "$(cat ${RUN_DIR}/api.pid)" 2>/dev/null; then
        tail -n 20 ${RUN_DIR}/api.log >&2
        rm -f ${RUN_DIR}/api.pid
        fail "a API encerrou durante a inicialização."
    fi
    if health=$(curl -fsS -m 15 ${API_URL}/api/health 2>/dev/null); then
        break
    fi
    sleep 1
done
[ -n "${health}" ] || fail "a API não respondeu em ${API_URL}/api/health; veja .run/api.log."

# ------------------------------------------------------------------------------
step "Estado dos componentes"
# ------------------------------------------------------------------------------

if command -v jq >/dev/null 2>&1; then
    echo "${health}" | jq -r '
        (.orgs[] | "\(.name)  Fabric: \(.fabric)\n      IPFS:   \(.ipfs)"),
        "Custodiantes (limiar \(.threshold) de \(.total)):",
        (.custodians[] | "  \(.address): \(.status)")'
    problems=$(echo "${health}" | jq '
        [.orgs[] | .fabric, .ipfs | select(startswith("ok") | not)] +
        [.custodians[] | .status | select(startswith("ativo") | not)] | length')
else
    echo "${health}"
    problems=$(echo "${health}" | grep -c "sem resposta\|indisponível" || true)
fi

echo ""
if [ "${problems}" -ne 0 ]; then
    echo "AVISO: ${problems} componente(s) sem resposta; a aba Rede do frontend detalha o estado."
fi
echo "Frontend disponível em ${API_URL}"
echo ""
echo "  Roteiro de demonstração: frontend/README.md"
echo "  Log da API:              .run/api.log"
echo "  Encerrar a API:          ./stop.sh"
echo "  Encerrar também as VMs:  ./stop.sh --vms"
