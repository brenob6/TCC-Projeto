# IPFS Private Network (PoC)

Prova de conceito de uma rede IPFS privada com três nós, referente ao item de
*backlog* **B-06**. Os nós usam o Kubo (implementação Go do IPFS) e
compartilham uma `swarm.key`, de modo que só se conectam entre si e ficam
isolados da rede IPFS pública.

Cada instituição de saúde opera o próprio nó IPFS na mesma máquina do seu
peer e do seu orderer. Por isso, este diretório não define máquinas: reúne os
scripts e os artefatos usados pelo `Vagrantfile` de `../fabric-network`, que o
monta em `/ipfs-private` nas máquinas.

## Topologia

| Máquina | IP | Instituição | Portas do nó IPFS |
|---------|----|-------------|-------------------|
| `fabric-node-1` | `192.168.56.31` | `Org1` | 4001 (swarm), 5001 (API), 8080 (gateway) |
| `fabric-node-2` | `192.168.56.32` | `Org2` | 4001 (swarm), 5001 (API), 8080 (gateway) |
| `fabric-node-3` | `192.168.56.33` | `Org3` | 4001 (swarm), 5001 (API), 8080 (gateway) |

## Como executar

Os nós sobem com a rede Fabric:

```bash
../fabric-network/scripts/start.sh
```

Ou com todo o ambiente, pelo `start.sh` da raiz do repositório.

Para verificar a conexão entre os nós:

```bash
cd ../fabric-network
vagrant ssh fabric-node-1 -c "ipfs swarm peers"
```

## Estrutura e scripts

| Arquivo | Descrição |
|---------|-----------|
| `download-ipfs.sh` | Baixa o Kubo `v0.42.0` (linux-amd64) para `downloads/kubo.tar.gz`. O download é feito no host para que as VMs não precisem baixar o binário individualmente. |
| `scripts/prepare.sh` | Executado no host antes de subir as máquinas. Executa `download-ipfs.sh`, se necessário, e gera a `swarm.key`, caso ainda não exista. |
| `scripts/provision.sh` | Provisionador `ipfs`, executado em cada máquina no `vagrant up`. Atualiza os pacotes, instala o Kubo a partir de `/ipfs-private/downloads/kubo.tar.gz` (se ainda não estiver instalado), executa `ipfs init`, remove os bootstrap peers públicos, copia a `swarm.key`, expõe API e gateway em `0.0.0.0`, desativa o `AutoConf` e cria/ativa o serviço systemd `ipfs`. Ao final, grava o Peer ID do nó em `peers/<hostname>.peer`. |
| `scripts/bootstrap.sh` | Provisionador `ipfs-bootstrap` (`run: "never"`), executado após o provisionamento das três máquinas. Aguarda os Peer IDs dos três nós, remove todos os bootstrap peers e adiciona como bootstrap os outros dois nós (`/ip4/<ip>/tcp/4001/p2p/<peer-id>`). Por fim, lista os peers conectados com `ipfs swarm peers`. |
| `swarm.key` | Gerado. Chave pré-compartilhada da rede privada. Todos os nós precisam da mesma chave para se comunicar. |
| `peers/` | Gerado. Contém o Peer ID de cada nó, usado pelo `bootstrap.sh` para montar a lista de bootstrap. |
| `downloads/` | Gerado pelo `download-ipfs.sh`. Contém o tarball do Kubo. |

## Fluxo de inicialização

1. `scripts/prepare.sh` disponibiliza o Kubo e a `swarm.key` no host.
2. `vagrant up` (em `../fabric-network`) cria as máquinas e executa o
   provisionador `ipfs` em cada uma, após o do Fabric.
3. Cada nó inicia o daemon IPFS com a `swarm.key` e publica seu Peer ID em
   `peers/`.
4. `vagrant provision --provision-with=ipfs-bootstrap` configura, em cada nó,
   os demais nós como bootstrap peers.

## Observações

- A `swarm.key` é material sensível. Não a versione nem a compartilhe fora do
  ambiente de testes.
- Ao recriar as máquinas, os Peer IDs mudam; os arquivos em `peers/` são
  sobrescritos no novo provisionamento.
- Os IPs dos nós estão definidos no `Vagrantfile` de `../fabric-network` e no
  `bootstrap.sh`. Ao alterar a topologia, atualize os dois arquivos.
