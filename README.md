# TCC-Projeto

## Execução completa

```sh
./start.sh
```

Sobe todos os módulos e, ao final, deixa o frontend disponível em
<http://127.0.0.1:8080>:

| Etapa | Módulo | Máquinas |
|---|---|---|
| 1 | `fabric-network` + `ipfs-private` + `prontuarios-cc` — orderer, peer e nó IPFS de cada instituição, e chaincode | `fabric-node-1..3` (`192.168.56.31-33`, 2 GB) |
| 2 | `key-sharing` — custodiantes de chaves, que confirmam cada decifração no livro-razão | `custodian-1..3` (`192.168.56.21-23`, 512 MB) |
| 3 | `frontend` — build estático | host |
| 4 | `api-gateway` — API em segundo plano, servindo o frontend | host |

Cada máquina `fabric-node-N` representa uma instituição e executa os serviços
que ela opera (orderer, peer, chaincode e nó IPFS). Os custodiantes ficam em
máquinas próprias, pois são entidades independentes das instituições
(Seção 5.5.1): nenhuma instituição guarda um compartilhamento de chave.
Os custodiantes sobem depois da rede Fabric porque cada um consulta o peer de
uma organização, com uma identidade cliente própria (`User2@orgN`), para
confirmar que a decifração solicitada corresponde a uma leitura autorizada e
registrada no livro-razão. O canal entre a API e os custodiantes usa TLS
mútuo (`key-sharing/build/tls`).

Na primeira execução o script baixa os binários, cria e provisiona as
máquinas, gera os artefatos da rede e implanta o chaincode. Nas seguintes,
apenas liga as máquinas paradas, repara a rede *host-only* quando necessário,
atualiza o chaincode e os custodiantes se o código mudou e reinicia a API.
Máquinas suspensas são desligadas de forma ordenada e reiniciadas, e máquinas
ligadas com memória diferente da definida no `Vagrantfile` são reiniciadas
para aplicá-la. Ao final, exibe o estado de cada componente.

Pré-requisitos: Vagrant, VirtualBox, Go 1.24, Node.js 24 com npm, `wget` e
`curl` (`jq` opcional, para o resumo final). As seis máquinas ocupam cerca de
7,5 GB de RAM.

| Comando | Efeito |
|---|---|
| `./stop.sh` | Encerra a API Gateway; as máquinas continuam ligadas |
| `./stop.sh --vms` | Encerra também as máquinas (`vagrant halt`), preservando os dados |
| `API_LISTEN=127.0.0.1:8081 ./start.sh` | Usa outro endereço para a API e o frontend |

O log da API fica em `.run/api.log`. Com o ambiente em execução,
`python3 avaliacao/medir.py` percorre os fluxos pela API e mede as
latências (`avaliacao/README.md`). O roteiro de demonstração está em
`frontend/README.md`, e os detalhes de cada módulo, no README do respectivo
diretório.
