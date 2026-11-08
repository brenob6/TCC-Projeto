# Rede Hyperledger Fabric (PoC)

Infraestrutura da rede *blockchain* descrita na proposta (seção *Rede
Blockchain*), referente ao item de *backlog* **B-01**: três máquinas Vagrant,
uma por instituição de saúde, executando o Hyperledger Fabric `2.5.16` (LTS)
com ordenação Raft e um canal compartilhado. Cada máquina executa também o nó
IPFS da instituição (B-06), provisionado com os scripts de `../ipfs-private`.

Esta etapa cobre **somente o setup da rede**. Nenhum *chaincode* é instalado e
nenhuma transação é submetida; os contratos B-02 a B-05 serão implantados
sobre esta infraestrutura.

## Topologia

| Máquina | IP | Orderer (Raft) | Peer | Organização |
|---------|----|----------------|------|-------------|
| `fabric-node-1` | `192.168.56.31` | `orderer1.example.com:7050` | `peer0.org1.example.com:7051` | `Org1MSP` |
| `fabric-node-2` | `192.168.56.32` | `orderer2.example.com:7050` | `peer0.org2.example.com:7051` | `Org2MSP` |
| `fabric-node-3` | `192.168.56.33` | `orderer3.example.com:7050` | `peer0.org3.example.com:7051` | `Org3MSP` |

Portas adicionais em cada máquina: `7053` (Admin API do orderer, *channel
participation*), `7052` (chaincode), `9443` e `9444` (*operations*/métricas do
orderer e do peer) e, do nó IPFS, `4001` (*swarm*), `5001` (API) e `8080`
(*gateway*).

- **Organizações:** `Org1MSP`, `Org2MSP` e `Org3MSP` representam as
  instituições; `OrdererMSP` agrupa os três orderers.
- **Consenso:** `etcdraft` com três consentidores, tolerando a falha de um
  orderer.
- **Canal:** `prontuarios`, com as três organizações e *anchor peers*
  definidos já no bloco gênese.
- **TLS:** habilitado em todas as conexões; a Admin API do orderer exige
  autenticação mútua.
- **Ledger:** LevelDB (padrão do peer).

Cada VM usa a box `bento/ubuntu-24.04` com 2 GB de RAM e 1 vCPU, em uma rede
`private_network` do VirtualBox. O diretório `../ipfs-private` é montado em
`/ipfs-private`, com o Kubo, a `swarm.key` e os Peer IDs dos nós. Os nós executam os binários nativos do Fabric
como serviços systemd, sem Docker; os *chaincodes* serão executados como
serviço externo (*chaincode-as-a-service*), cujo *builder* já está instalado em
`/opt/hyperledger/ccaas_builder`.

## Pré-requisitos

- Vagrant
- VirtualBox
- `wget` (para baixar os binários do Fabric)
- Host Linux `amd64` (os mesmos binários são usados no host para gerar os
  artefatos)

## Como executar

```bash
./scripts/start.sh
```

O script baixa os binários (se necessário), gera os artefatos, sobe as VMs,
cria o canal e interliga os nós IPFS. Equivale a:

```bash
./download-fabric.sh                               # uma única vez
../ipfs-private/scripts/prepare.sh                 # Kubo e swarm.key (uma única vez)
./scripts/generate.sh                              # material criptográfico e bloco gênese
vagrant up                                         # orderer, peer e nó IPFS em cada máquina
vagrant provision --provision-with=channel         # ingresso no canal
vagrant provision --provision-with=ipfs-bootstrap  # malha da rede IPFS privada
```

Para verificar o estado da rede:

```bash
./scripts/status.sh
```

Resultado esperado: os três orderers com `"consensusRelation": "consenter"` e
`"status": "active"`, e os três peers com `height` 1 e o mesmo
`currentBlockHash` (o bloco gênese do canal).

## Estrutura e scripts

| Arquivo | Descrição |
|---------|-----------|
| `Vagrantfile` | Define as três VMs e os provisionadores: `provision.sh` e `ipfs` (`../ipfs-private/scripts/provision.sh`), executados no `vagrant up`, e `channel` (`channel.sh`) e `ipfs-bootstrap` (`../ipfs-private/scripts/bootstrap.sh`), com `run: "never"`, executados sob demanda. |
| `download-fabric.sh` | Baixa o Fabric `v2.5.16` (linux-amd64) para `downloads/fabric.tar.gz`. |
| `config/crypto-config.yaml` | Organizações e nós para o `cryptogen`, com os IPs incluídos como SANs dos certificados. |
| `config/configtx.yaml` | Organizações, políticas, consentidores Raft e o perfil `ProntuariosChannel`. |
| `scripts/start.sh` | Ponto de entrada; executa todos os passos acima. |
| `scripts/generate.sh` | Executado no host. Gera `organizations/` (`cryptogen`) e `channel-artifacts/prontuarios.block` (`configtxgen`). Não sobrescreve artefatos existentes. |
| `scripts/provision.sh` | Executado em cada VM com o índice do nó. Configura `/etc/hosts`, instala os binários e o *builder* ccaas, copia somente o material criptográfico daquela máquina para `/etc/hyperledger/fabric/organizations` e registra os serviços `fabric-orderer` e `fabric-peer`. |
| `scripts/channel.sh` | Executado em cada VM. Ingressa o orderer no canal pela Admin API (`osnadmin channel join`) e o peer pelo bloco gênese (`peer channel join`). É idempotente. |
| `scripts/status.sh` | Executado no host. Consulta a participação de cada orderer e a altura do ledger de cada peer. |
| `organizations/` | Gerado. Certificados e chaves das organizações. |
| `channel-artifacts/` | Gerado. Bloco gênese do canal. |
| `downloads/` | Gerado. *Tarball* e binários do Fabric. |

## Operação nas máquinas

Cada VM define em `/etc/profile.d/fabric.sh` as variáveis do administrador da
sua organização (`CORE_PEER_*`, `ORDERER_CA`, `ORDERER_ADMIN_*`,
`CHANNEL_NAME`), de modo que os comandos `peer` e `osnadmin` funcionam
diretamente em uma sessão `vagrant ssh`:

```bash
vagrant ssh fabric-node-1
peer channel list
peer channel getinfo -c $CHANNEL_NAME
osnadmin channel list -o $ORDERER_ADMIN_ADDRESS --ca-file $ORDERER_CA \
    --client-cert $ORDERER_ADMIN_TLS_CERT --client-key $ORDERER_ADMIN_TLS_KEY
```

Serviços e dados:

```bash
sudo systemctl status fabric-orderer fabric-peer ipfs
sudo journalctl -u fabric-orderer -u fabric-peer -f
ls /var/hyperledger/production/{orderer,peer}
ipfs swarm peers                                   # os outros dois nós IPFS
```

## Observações

- `organizations/` contém chaves privadas de todas as organizações, e
  `.vagrant/` contém as chaves SSH das VMs. Não os versione nem os compartilhe
  fora do ambiente de testes.
- Em uma rede real, cada instituição geraria o próprio material
  criptográfico (por exemplo, com Fabric CA). O `cryptogen` centralizado no
  host é uma simplificação da prova de conceito; para reduzir a exposição,
  cada VM recebe apenas os certificados e chaves de que precisa.
- Durante o `channel`, enquanto menos de dois orderers participam do canal, os
  logs exibem avisos de `SERVICE_UNAVAILABLE`/`NOT_FOUND` e falhas de envio do
  Raft. Eles cessam assim que o líder é eleito.
- Para recriar a rede do zero: `vagrant destroy -f`, remova `organizations/` e
  `channel-artifacts/` e execute `./scripts/start.sh` novamente.
- Os IPs estão definidos no `Vagrantfile`, no `provision.sh` (`/etc/hosts`) e
  nos SANs de `config/crypto-config.yaml`. Ao alterar a topologia, atualize os
  três arquivos.
