# IPFS Private Network (PoC)

Prova de conceito de uma rede IPFS privada com três nós, provisionada com Vagrant e VirtualBox. Os nós usam o Kubo (implementação Go do IPFS) e compartilham uma `swarm.key`, de modo que só se conectam entre si e ficam isolados da rede IPFS pública.

## Topologia

| Nó            | IP              | Portas expostas               |
|---------------|-----------------|-------------------------------|
| `ipfs-node-1` | `192.168.56.11` | 4001 (swarm), 5001 (API), 8080 (gateway) |
| `ipfs-node-2` | `192.168.56.12` | 4001 (swarm), 5001 (API), 8080 (gateway) |
| `ipfs-node-3` | `192.168.56.13` | 4001 (swarm), 5001 (API), 8080 (gateway) |

Cada VM usa a box `bento/ubuntu-24.04` com 1 GB de RAM e 1 vCPU, em uma rede `private_network` do VirtualBox. O diretório do projeto é montado em `/vagrant` dentro de cada VM.

## Pré-requisitos

- Vagrant
- VirtualBox
- `wget` (para baixar o binário do Kubo)

## Como executar

```bash
# 1. Baixar o Kubo (executar uma única vez)
sh download-ipfs.sh

# 2. Subir as VMs e conectar os nós entre si
bash scripts/start.sh
```

Para verificar a conexão entre os nós:

```bash
vagrant ssh ipfs-node-1 -c "ipfs swarm peers"
```

## Estrutura e scripts

| Arquivo | Descrição |
|---------|-----------|
| `Vagrantfile` | Define as três VMs, seus IPs e os dois provisionadores: `provision.sh` (executado no `vagrant up`) e `bootstrap` (`bootstrap.sh`, marcado como `run: "never"`, ou seja, só roda quando chamado explicitamente). |
| `download-ipfs.sh` | Baixa o Kubo `v0.42.0` (linux-amd64) para `downloads/kubo.tar.gz`. O download é feito no host para que as VMs não precisem baixar o binário individualmente. |
| `scripts/start.sh` | Ponto de entrada. Executa `vagrant up` (sobe e provisiona as três VMs) e, em seguida, `vagrant provision --provision-with=bootstrap` para interligar os nós. |
| `scripts/provision.sh` | Executado em cada VM. Atualiza os pacotes, instala o Kubo a partir de `/vagrant/downloads/kubo.tar.gz` (se ainda não estiver instalado), executa `ipfs init`, remove os bootstrap peers públicos, copia a `swarm.key`, expõe API e gateway em `0.0.0.0`, desativa o `AutoConf` e cria/ativa o serviço systemd `ipfs`. Ao final, grava o Peer ID do nó em `/vagrant/peers/<hostname>.peer`. |
| `scripts/bootstrap.sh` | Executado em cada VM após o provisionamento. Aguarda os três arquivos `.peer` existirem, remove todos os bootstrap peers e adiciona como bootstrap os outros dois nós (`/ip4/<ip>/tcp/4001/p2p/<peer-id>`). Por fim, lista os peers conectados com `ipfs swarm peers`. |
| `swarm.key` | Chave pré-compartilhada da rede privada. Todos os nós precisam da mesma chave para se comunicar. |
| `peers/` | Gerado automaticamente. Contém o Peer ID de cada nó, usado pelo `bootstrap.sh` para montar a lista de bootstrap. |
| `downloads/` | Gerado pelo `download-ipfs.sh`. Contém o tarball do Kubo. |
| `.vagrant/` | Estado interno do Vagrant (não editar). |

## Fluxo de inicialização

1. `vagrant up` cria as VMs e executa `provision.sh` em cada uma.
2. Cada nó inicia o daemon IPFS com a `swarm.key` e publica seu Peer ID em `peers/`.
3. `vagrant provision --provision-with=bootstrap` executa `bootstrap.sh` em cada nó, que configura os demais nós como bootstrap peers.

## Observações

- A `swarm.key` e as chaves em `.vagrant/` são material sensível. Não as versione nem as compartilhe fora do ambiente de testes.
- Ao recriar as VMs (`vagrant destroy` seguido de `vagrant up`), os Peer IDs mudam; os arquivos em `peers/` são sobrescritos no novo provisionamento.
- Os IPs dos nós estão definidos tanto no `Vagrantfile` quanto no `bootstrap.sh`. Ao alterar a topologia, atualize os dois arquivos.
