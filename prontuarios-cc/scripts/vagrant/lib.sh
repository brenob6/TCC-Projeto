# Funções comuns aos scripts executados no host, que operam as máquinas da
# rede de ../../fabric-network por meio do Vagrant.

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
NETWORK=$(cd ${ROOT}/../fabric-network && pwd)
STAGE=${NETWORK}/chaincode

# Os IPs repetem os definidos em ../../fabric-network/Vagrantfile.
NODES=(fabric-node-1 fabric-node-2 fabric-node-3)
IPS=(192.168.56.31 192.168.56.32 192.168.56.33)

# vm <máquina> <comando>: executa um comando na máquina, sem as mensagens do
# ssh.
vm() {
    (cd ${NETWORK} && vagrant ssh "$1" -c "$2" -- -q)
}

# on_all <comando>: executa o comando nas três máquinas, em sequência.
on_all() {
    local node
    for node in "${NODES[@]}"; do
        vm ${node} "$1"
    done
}

# on_all_parallel <comando>: executa o comando nas três máquinas ao mesmo
# tempo e falha se alguma delas falhar.
on_all_parallel() {
    local node pids=() failed=0
    for node in "${NODES[@]}"; do
        vm ${node} "$1" &
        pids+=($!)
    done
    for pid in "${pids[@]}"; do
        wait ${pid} || failed=1
    done
    return ${failed}
}

# ensure_running: liga ou retoma as máquinas que não estiverem em execução,
# sem reprovisioná-las.
ensure_running() {
    local stopped
    stopped=$(cd ${NETWORK} && vagrant status --machine-readable \
        | awk -F, '$3 == "state" && $4 != "running" {print $2}')
    if [ -n "${stopped}" ]; then
        echo "Iniciando: ${stopped}"
        (cd ${NETWORK} && vagrant up --no-provision ${stopped})
    fi
}

# mesh_ok: verifica se cada máquina alcança as demais pela rede privada.
mesh_ok() {
    local targets="${IPS[*]}"
    on_all "for ip in ${targets}; do ping -c1 -W2 \$ip >/dev/null || exit 1; done"
}

# repair_network: reativa o enlace do adaptador da rede privada e limpa a
# tabela ARP. Necessário quando o VirtualBox restaura máquinas salvas e a rede
# host-only fica parcialmente inacessível (Raft sem líder).
repair_network() {
    local node id
    for node in "${NODES[@]}"; do
        id=$(cat ${NETWORK}/.vagrant/machines/${node}/virtualbox/id)
        VBoxManage controlvm ${id} setlinkstate2 off
        VBoxManage controlvm ${id} setlinkstate2 on
    done
    sleep 3
    on_all "sudo ip neigh flush dev eth1"
}

# ensure_network: garante a conectividade entre as máquinas.
ensure_network() {
    if mesh_ok; then
        return
    fi
    echo "Rede privada com falhas; reativando os enlaces..."
    repair_network
    if ! mesh_ok; then
        echo "ERRO: as máquinas continuam sem comunicação entre si." >&2
        exit 1
    fi
}

# wait_orderers: aguarda cada orderer entregar blocos, o que exige um líder
# Raft eleito.
wait_orderers() {
    local node
    for node in "${NODES[@]}"; do
        vm ${node} '
            source /etc/profile.d/fabric.sh
            for _ in $(seq 1 30); do
                peer channel fetch newest /tmp/newest.block -c ${CHANNEL_NAME} \
                    -o ${ORDERER_ADDRESS} --tls --cafile ${ORDERER_CA} >/dev/null 2>&1 && exit 0
                sleep 2
            done
            echo "ERRO: ${ORDERER_ADDRESS} sem líder Raft." >&2
            exit 1'
    done
}

# prepare_machines: deixa a rede pronta para receber o chaincode.
prepare_machines() {
    ensure_running
    ensure_network
    wait_orderers
}

# stage: compila e copia os artefatos para a pasta montada em
# /vagrant/chaincode nas máquinas.
stage() {
    ${ROOT}/scripts/build.sh
    rm -rf ${STAGE}
    mkdir -p ${STAGE}
    cp ${ROOT}/build/{prontuarios-cc,patient-signer,prontuarios-cc.tar.gz,package-id,pepper} ${STAGE}/
    cp ${ROOT}/scripts/fabric/*.sh ${STAGE}/
    chmod 600 ${STAGE}/pepper
}

# verify: confere o estado do chaincode nas três máquinas.
verify() {
    local expected report
    expected=$(sha256sum ${ROOT}/build/prontuarios-cc | cut -c1-16)
    report=$(on_all "bash /vagrant/chaincode/check.sh")
    echo "${report}"
    if [ "$(echo "${report}" | grep -c "servico=active binario=${expected} pacote=ok")" -ne ${#NODES[@]} ]; then
        echo "ERRO: as máquinas não estão todas com o chaincode atualizado (binário esperado: ${expected})." >&2
        return 1
    fi
    echo "Chaincode atualizado e ativo nas ${#NODES[@]} máquinas."
}
