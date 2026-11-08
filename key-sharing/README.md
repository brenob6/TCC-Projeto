# Compartilhamento de chaves com custodiantes via gRPC

Prova de conceito do mecanismo de gerenciamento de chaves descrito na proposta
(seções *Gerenciamentos de chaves* e *Custodiantes de Chaves*), referente aos
itens de *backlog* **B-07** e **B-08**.

Cada paciente possui uma chave criptográfica cujo segredo é dividido pelo
esquema Shamir Secret Sharing e distribuído entre Custodiantes de Chaves
distintos. O KMS coordena a geração, a distribuição e a recuperação, enquanto
cada custodiante expõe uma interface gRPC e mantém seus compartilhamentos em uma
base SQLite local.

## Propriedade central

A chave privada **nunca é reconstituída**, em nenhuma entidade e em nenhum
momento. Cada custodiante calcula apenas uma decifração parcial
`D_i = C1^{x_i}` com o compartilhamento sob sua custódia; o KMS combina as
parciais por interpolação de Lagrange no expoente, obtendo `D = C1^x = Y^r` sem
jamais aprender `x`. O compartilhamento não trafega pela rede.

Consequências:

- Um custodiante isolado, ainda que comprometido, não decifra prontuário algum.
- São necessários pelo menos `k` custodiantes para qualquer recuperação.
- Com `n > k`, a indisponibilidade de custodiantes não interrompe o atendimento.

## Componentes

| Caminho | Papel |
| --- | --- |
| `proto/custodian.proto` | Contrato gRPC do custodiante |
| `internal/crypto` | Esquema de limiar (Shamir + ElGamal + AES-256-GCM) |
| `internal/custodian` | Serviço gRPC e persistência SQLite |
| `internal/kms` | Cliente que coordena os custodiantes |
| `cmd/custodian` | Binário do Custodiante de Chaves |
| `cmd/kms` | Interface de linha de comando do KMS |
| `kmsclient` | Reexporta o KMS para módulos externos, como `../api-gateway` |

O esquema criptográfico reaproveita o utilizado em `../kms`, aqui distribuído
entre serviços independentes.

Diretórios auxiliares: `pb/` contém o código Go gerado a partir do `.proto`,
`scripts/` reúne os scripts de compilação, provisionamento e testes, e `build/`
recebe os binários compilados (ignorado pelo Git).

### Interface gRPC do custodiante

| RPC | Função |
| --- | --- |
| `StoreShare` | Recebe e persiste o compartilhamento de uma chave |
| `PartialDecrypt` | Calcula `D_i = C1^{x_i}` e registra a operação na auditoria |
| `Health` | Informa a disponibilidade do custodiante |

Cada custodiante mantém uma base SQLite com duas tabelas: `shares`
(compartilhamento por `key_id`) e `access_log` (solicitante, instituição,
caráter emergencial, justificativa e momento da operação).

### Cifração de um prontuário

O prontuário é protegido por ElGamal híbrido sobre a curva Ed25519:

```
C1         = g^r                        ponto efêmero
shared     = Y^r                        segredo compartilhado
K          = chave AES-256 aleatória
wrappedKey = K XOR SHA256(shared)
ciphertext = AES-256-GCM(K, prontuário)
```

Somente o envelope (`C1`, `wrappedKey`, `nonce`, `ciphertext`) é armazenado —
na arquitetura final, na rede IPFS. A chave simétrica `K` não é persistida.

## Pré-requisitos

- Go 1.24 (versão fixada em `.tool-versions`)
- Vagrant com VirtualBox (box `bento/ubuntu-24.04`), apenas para o ambiente
  distribuído
- `protoc`, apenas para regenerar o código gRPC

## Scripts

| Script | Onde executa | Função |
| --- | --- | --- |
| `scripts/start.sh` | Host | Compila, sobe as máquinas e executa os fluxos de operação |
| `scripts/build.sh` | Host | Compila `custodian` e `kms` estáticos para `linux/amd64` em `build/` |
| `scripts/provision.sh` | VM custodiante | Instala o binário, o `sqlite3` e o serviço systemd `custodian` (porta 50051, base em `/var/lib/custodian/shares.db`) |
| `scripts/provision-kms.sh` | VM `kms` | Instala o binário e define `CUSTODIANS` e `THRESHOLD` em `/etc/profile.d/kms.sh` |
| `scripts/test.sh` | VM `kms` | Fluxos de ponta a ponta: `health`, `keygen`, `encrypt`, recuperação autorizada, recuperação extraordinária |
| `scripts/test-tolerancia.sh` | Host | Para e reinicia custodiantes para validar o limiar e exibe a auditoria de cada nó |

Os scripts de provisionamento são acionados pelo `Vagrantfile`; `test.sh` é um
provisionador nomeado (`test`) com `run: "never"`, executado apenas sob demanda.
`test.sh` e `test-tolerancia.sh` aceitam a variável `KEY_ID` (padrão
`paciente-001`); `test-tolerancia.sh` depende de `test.sh` já ter sido executado.

## Ambiente de testes

O ambiente Vagrant reproduz quatro máquinas em rede privada:

| Máquina | Endereço | Papel |
| --- | --- | --- |
| `custodian-1` | `192.168.56.21:50051` | Custodiante de Chaves |
| `custodian-2` | `192.168.56.22:50051` | Custodiante de Chaves |
| `custodian-3` | `192.168.56.23:50051` | Custodiante de Chaves |
| `kms` | `192.168.56.20` | Serviço de gerenciamento de chaves |

O esquema padrão é **2-de-3**: dois custodiantes bastam para recuperar um
prontuário e a queda de um deles não interrompe o atendimento.

Os binários são compilados no host (`CGO_ENABLED=0`, estáticos) e instalados nas
máquinas pela pasta compartilhada, dispensando a instalação do Go nas VMs.

### Execução

```sh
./scripts/start.sh
```

O script compila os binários, sobe as máquinas e executa os fluxos de operação.
Equivale a:

```sh
./scripts/build.sh
vagrant up
vagrant provision kms --provision-with=test
```

Para validar a tolerância a falhas e inspecionar a auditoria — o script usa o
comando `vagrant` para parar e reiniciar custodiantes, portanto roda no host:

```sh
./scripts/test-tolerancia.sh
```

### Testes manuais

Os passos abaixo reproduzem, um a um, os fluxos de `scripts/test.sh` e permitem
observar o estado de cada máquina. Pressupõem o ambiente já criado com
`./scripts/build.sh` e `vagrant up`. Os comandos indicados como **host** rodam
no diretório do projeto; os indicados como **kms** rodam dentro da máquina
`kms`.

#### 1. Verificar os custodiantes

**kms**

```sh
vagrant ssh kms
cd ~/kms        # keys/ e os arquivos cifrados são gravados no diretório corrente
kms health
```

Os três custodiantes devem aparecer como disponíveis no esquema 2-de-3. Os
endereços e o limiar vêm das variáveis `CUSTODIANS` e `THRESHOLD`, definidas em
`/etc/profile.d/kms.sh`.

#### 2. Gerar a chave e observar os shares nos custodiantes

**kms**

```sh
kms keygen paciente-002
cat keys/paciente-002.pub.json     # apenas a chave pública fica no KMS
```

**host** — consulta a tabela `shares` de cada custodiante:

```sh
for node in custodian-1 custodian-2 custodian-3; do
  echo "[$node]"
  vagrant ssh "$node" -c "sudo sqlite3 -header -column /var/lib/custodian/shares.db \
    'SELECT key_id, share_index, length(share_value) AS bytes,
            hex(share_value) AS share_value, created_at FROM shares'"
done
```

Resultado esperado:

- cada custodiante possui exatamente uma linha para `paciente-002`;
- o `share_index` é distinto em cada nó (`0`, `1` e `2`; a biblioteca Kyber
  numera os compartilhamentos a partir de zero) e o `share_value` difere entre
  eles;
- nenhum valor isolado corresponde à chave privada, que não é armazenada em
  máquina alguma.

Para consultar um único custodiante de forma interativa:

```sh
vagrant ssh custodian-1
sudo sqlite3 /var/lib/custodian/shares.db
sqlite> .headers on
sqlite> .mode column
sqlite> SELECT key_id, share_index, hex(share_value) FROM shares;
sqlite> .quit
```

#### 3. Cifrar um arquivo

**kms**

```sh
printf 'Paciente: 002\nDiagnóstico: diabetes tipo 2\n' > prontuario.txt
kms encrypt paciente-002 prontuario.txt
cat prontuario.txt.enc
```

O arquivo `prontuario.txt.enc` é um JSON com os campos `c1`, `wrapped_key`,
`nonce` e `ciphertext` (codificados em Base64). A cifração usa somente a chave
pública e não contata os custodiantes, portanto não gera registros de
auditoria.

#### 4. Decifrar o arquivo no KMS

**kms**

```sh
kms -requester medico-001 -institution hospital-a \
    decrypt paciente-002 prontuario.txt.enc

cat decifrado_prontuario.txt
diff prontuario.txt decifrado_prontuario.txt && echo "OK: conteúdo idêntico"
```

O KMS solicita a decifração parcial `D_i = C1^{x_i}` aos custodiantes,
combina `k` respostas e grava o resultado em `decifrado_<arquivo>` no
diretório corrente. O comando falha caso o arquivo `.enc` tenha sido alterado
(a autenticação do AES-GCM detecta a adulteração) ou caso o `key-id` não
corresponda à chave usada na cifração.

**host** — confirma o registro da operação na auditoria:

```sh
vagrant ssh custodian-1 -c "sudo sqlite3 -header -column /var/lib/custodian/shares.db \
  'SELECT key_id, requester_id, institution_id, emergency, reason, occurred_at
   FROM access_log ORDER BY id DESC LIMIT 5'"
```

#### 5. Recuperação extraordinária (RF-06)

**kms**

```sh
rm -f decifrado_prontuario.txt
kms -requester medico-plantao -institution hospital-emergencia \
    -reason "paciente inconsciente" \
    emergency paciente-002 prontuario.txt.enc
```

Repita a consulta à tabela `access_log`: a nova linha apresenta
`emergency = 1` e a justificativa informada em `reason`.

#### 6. Tolerância a falhas

**host**

```sh
vagrant ssh custodian-3 -c "sudo systemctl stop custodian"
vagrant ssh kms -c "cd ~/kms && kms decrypt paciente-002 prontuario.txt.enc"   # sucesso: 2 de 3

vagrant ssh custodian-2 -c "sudo systemctl stop custodian"
vagrant ssh kms -c "cd ~/kms && kms decrypt paciente-002 prontuario.txt.enc"   # falha: 1 de 3

vagrant ssh custodian-2 -c "sudo systemctl start custodian"
vagrant ssh custodian-3 -c "sudo systemctl start custodian"
```

Com um único custodiante ativo, abaixo do limiar `k = 2`, o prontuário
permanece inacessível. O script `scripts/test-tolerancia.sh` automatiza essa
verificação.

### Referência da CLI

`kms [opções] <comando> [argumentos]`

| Opção | Padrão | Descrição |
| --- | --- | --- |
| `-custodians` | `$CUSTODIANS` ou `localhost:50051` | Endereços gRPC separados por vírgula |
| `-k` | `$THRESHOLD` ou `2` | Limiar `k` do esquema `(k, n)` |
| `-requester` | `medico-001` | Identificador do solicitante, registrado na auditoria |
| `-institution` | `hospital-a` | Instituição de origem |
| `-reason` | vazio | Justificativa da recuperação extraordinária |

| Comando | Efeito |
| --- | --- |
| `health` | Consulta o estado dos custodiantes |
| `keygen <key-id>` | Gera a chave, distribui os compartilhamentos e salva a chave pública em `keys/<key-id>.pub.json` |
| `encrypt <key-id> <arquivo>` | Gera `<arquivo>.enc` com o envelope cifrado |
| `decrypt <key-id> <arquivo.enc>` | Recupera o prontuário em `decifrado_<arquivo>` |
| `emergency <key-id> <arquivo.enc>` | Recuperação extraordinária; sem `-reason`, a CLI aplica uma justificativa padrão |

`custodian -id <nome> -listen <endereço> -db <caminho>` inicia um custodiante
(padrões: nome da máquina, `0.0.0.0:50051` e `shares.db`).

### Auditoria

Cada custodiante registra localmente toda decifração parcial que executa,
identificando solicitante, instituição, caráter emergencial e justificativa. O
acesso é negado caso o registro de auditoria falhe.

```sh
vagrant ssh custodian-1 -c \
  "sudo sqlite3 -header -column /var/lib/custodian/shares.db \
   'SELECT * FROM access_log'"
```

## Testes automatizados

```sh
go test ./...
go test -race ./...
```

Os testes de `internal/kms` sobem custodiantes gRPC reais em portas locais e
cobrem: fluxo completo, recuperação com um custodiante indisponível, recusa
abaixo do limiar, registro de auditoria da recuperação extraordinária, acesso
emergencial sem justificativa e chave inexistente. Os de `internal/crypto`
cobrem o esquema de limiar, incluindo a recusa de compartilhamentos de outra
chave e a detecção de prontuários adulterados.

## Escopo

Esta prova de conceito cobre o gerenciamento de chaves. Não integra a rede
Hyperledger Fabric (B-01 a B-05) nem o armazenamento IPFS (B-06): a validação
das permissões na *blockchain* e a persistência dos envelopes na rede
distribuída ocorrem fora deste componente.

O canal gRPC opera sem TLS e sem autenticação mútua, o que simplifica a execução
dos testes. Em produção, a comunicação entre KMS e custodiantes exige mTLS, e a
autorização de cada decifração parcial deve ser vinculada à permissão registrada
na *blockchain*.

## Requisitos atendidos

- **RF-05** — apenas quem reúne `k` decifrações parciais acessa o prontuário;
  a chave privada não existe de forma integral em nenhuma entidade.
- **RF-06** — a recuperação extraordinária exige justificativa e fica
  registrada em todos os custodiantes que participaram da operação.
- **RF-03 / RF-08** — cada custodiante mantém histórico com solicitante,
  instituição, tipo de acesso e momento da operação.

## Regeneração do código gRPC

Necessária apenas ao alterar `proto/custodian.proto`:

```sh
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.6
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

protoc --go_out=. --go_opt=module=poc-key-sharing \
       --go-grpc_out=. --go-grpc_opt=module=poc-key-sharing \
       proto/custodian.proto
```
