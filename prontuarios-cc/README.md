# Chaincode de prontuários (cc-tools)

Contratos inteligentes descritos na proposta (seção *Rede Blockchain*),
referentes aos itens de *backlog* **B-02** a **B-05**: cadastro de
instituições, pacientes e profissionais, registro e recuperação de
prontuários, controle de acesso por consentimento e recuperação
extraordinária. O chaincode usa o [cc-tools](https://github.com/hyperledger-labs/cc-tools)
`v1.0.3` e é implantado como serviço externo (CCaaS) na rede de
`../fabric-network`.

O ledger guarda apenas metadados, referências e a trilha de auditoria. O
conteúdo dos prontuários fica cifrado no IPFS (B-06) e a chave de cifração é
custodiada pelo KMS (B-07/B-08).

## Princípios

- **Sem dado pessoal em claro.** O paciente é identificado por um UUID
  derivado do `HMAC-SHA256(segredo do consórcio, CPF)`. O CPF chega pelo mapa
  transiente (`@request`) e não é gravado no bloco nem no estado.
- **Consentimento assinado pelo paciente.** Concessão, revogação e eliminação
  exigem assinatura Ed25519 do paciente sobre uma mensagem canônica, com
  *nonce* sequencial contra repetição. A API da instituição não consegue forjar
  nem alterar um consentimento.
- **Toda leitura é auditada.** `requestRecordAccess` é uma transação de
  escrita: leituras autorizadas, emergenciais e negadas ficam no ledger.
- **Escrita exige consentimento.** Registrar um prontuário exige permissão
  `leituraEscrita` do paciente.
- **Acesso extraordinário de uso único**, válido por 1 h, restrito a um
  prontuário e consumido na primeira leitura.
- **Validação de chamador.** O profissional informado deve pertencer à
  instituição da organização que assinou a transação.
- **Compatível com LevelDB.** As listagens usam o índice de referências do
  cc-tools (chaves compostas), sem consultas ricas do CouchDB.

## Ativos

| Ativo | Chave | Conteúdo |
|---|---|---|
| `institution` | `id` (CNES) | nome, CNPJ, `mspId`, situação |
| `patient` | `id` (UUID) | `cpfHash`, `kmsKeyId`, chave pública de cifração, chave pública de assinatura, `signatureNonce`, situação |
| `professional` | `id` (UUID) | `cpfHash`, nome, categoria, registro no conselho, instituição, habilitação emergencial, situação |
| `medicalRecord` | `cid` | hash do envelope cifrado, paciente, autor, instituição, tipo, `kmsKeyId`, eliminação lógica |
| `accessPermission` | `id` (txID) | paciente, profissional **ou** instituição, nível, escopo, finalidade, validade, situação, assinatura do paciente |
| `emergencyAccess` | `id` (txID) | paciente, profissional, instituição, prontuário, justificativa, validade, situação |
| `accessLog` | `id` (txID) | operação, paciente, prontuário, responsável, instituição, MSP, permissão ou acesso extraordinário, momento |

## Transações

| Item | Transação | Tipo | Função |
|---|---|---|---|
| B-02 | `registerInstitution` | escrita | Cadastra a instituição do MSP chamador |
| B-02 | `setInstitutionStatus` | escrita | Ativa/desativa a própria instituição |
| B-02 | `registerPatient` | escrita | Cadastra paciente (CPF transiente) |
| B-02 | `getPatientByCpf` | consulta | Localiza o paciente pelo CPF (transiente) |
| B-02 | `registerProfessional` | escrita | Vincula profissional à instituição chamadora |
| B-02 | `setProfessionalStatus` | escrita | Encerra/restabelece vínculo e habilitação emergencial |
| B-03 | `registerMedicalRecord` | escrita | Registra o CID (Figura 7); exige consentimento de escrita |
| B-03 | `requestRecordAccess` | escrita | Decide e audita a leitura (Figuras 8 e 9) |
| B-03 | `getAccessAuthorization` | consulta | Permite aos custodiantes confirmar a autorização antes de decifrar (inclui a justificativa do acesso extraordinário) |
| B-03 | `listPatientRecords` | consulta | Prontuários acessíveis ao profissional |
| B-03 | `getAccessHistory` | consulta | Histórico de acessos (RF-08) |
| B-03 | `invalidateMedicalRecord` | escrita | Eliminação lógica assinada pelo paciente |
| B-04 | `grantPermission` | escrita | Concessão assinada pelo paciente |
| B-04 | `revokePermission` | escrita | Revogação assinada pelo paciente |
| B-04 | `listPermissions` | consulta | Permissões com situação efetiva |
| B-04 | `checkPermission` | consulta | Verificação sem auditoria, para a interface |
| B-05 | `requestEmergencyAccess` | escrita | Registra justificativa e cria permissão de uso único |
| B-05 | `listEmergencyAccesses` | consulta | Acessos extraordinários do paciente |

`createAsset`, `updateAsset` e `deleteAsset` não são expostas. Permanecem as
consultas básicas do cc-tools (`readAsset`, `readAssetHistory`, `getSchema`,
`getDataTypes`, `getTx`, `getHeader`).

### Mensagem assinada pelo paciente

Objeto JSON com chaves em ordem alfabética, sem espaços e sem escape de HTML,
assinado com Ed25519 (assinatura em Base64 no argumento `signature`):

```json
{"accessLevel":"leitura","action":"grantPermission","domain":"prontuarios-cc/v1","expiresAt":"","institution":"","nonce":1,"patient":"<id>","professional":"<id>","purpose":"acompanhamento clínico","records":[]}
```

| Ação | Campos específicos |
|---|---|
| `grantPermission` | `professional`, `institution` (id ou `""`), `accessLevel`, `purpose`, `records` (CIDs em ordem), `expiresAt` (RFC 3339 ou `""`) |
| `revokePermission` | `permission` (id) |
| `invalidateMedicalRecord` | `record` (CID), `reason` |

O `nonce` deve ser `signatureNonce + 1`. `txdefs.SignedMessage` monta a
mensagem, e `cmd/patient-signer` gera chaves e assinaturas para testes.

## Estrutura

| Caminho | Conteúdo |
|---|---|
| `chaincode/assettypes` | Definição dos ativos |
| `chaincode/datatypes` | CPF, CNPJ, SHA-256, chave Ed25519 e enumerações |
| `chaincode/txdefs` | Transações, assinatura e avaliação de permissões |
| `chaincode/cmd/patient-signer` | Carteira simulada do paciente |
| `chaincode/main_test.go`, `chaincode/txdefs_test.go` | Testes com o *mock stub* do cc-tools |
| `scripts/build.sh` | Binários estáticos e pacote CCaaS determinístico em `build/` |
| `scripts/vagrant/` | Executados no **host**; operam as máquinas pelo Vagrant |
| `scripts/fabric/` | Executados **dentro das máquinas**; usam o CLI do Fabric e o systemd |

### Scripts do host (`scripts/vagrant/`)

| Script | Função |
|---|---|
| `deploy.sh` | Primeira implantação: instala, aprova, inicia o serviço e confirma a definição |
| `update.sh` | Atualiza o chaincode em uma rede já implantada (ver abaixo) |
| `lib.sh` | Funções comuns: liga ou retoma as máquinas, repara a rede privada, aguarda o líder Raft, distribui os artefatos e verifica o resultado |

### Scripts das máquinas (`scripts/fabric/`)

| Script | Função |
|---|---|
| `common.sh` | Variáveis do canal e consultas ao ciclo de vida |
| `install.sh` | Instala o pacote no peer e aprova a definição para a organização local |
| `service.sh` | `prepare` copia o binário sem reiniciar; `activate` substitui e reinicia o serviço |
| `commit.sh` | Confirma a definição no canal (endosso das três organizações) |
| `check.sh` | Estado do chaincode na máquina: serviço, binário, pacote, sequência e versão |
| `cc.sh` | Submete transações e consultas ao chaincode |

## Testes

```sh
cd chaincode
go test ./...
go test -race ./...
```

Os testes simulam as três organizações sobre o mesmo estado e cobrem:
cadastro e validações, ausência do CPF no estado, fluxo regular com
consentimento, permissões por instituição e por escopo, validade, assinatura
adulterada, chave errada e repetição de *nonce*, eliminação lógica, validação
de chamador, acesso extraordinário (justificativa, habilitação, uso único e
expiração) e bloqueio das transações genéricas de escrita.

## Implantação na rede Vagrant

Pré-requisitos: rede de `../fabric-network` criada (`./scripts/start.sh`
naquele diretório) e Go 1.24 no host. Os scripts ligam ou retomam as
máquinas, se necessário.

### Primeira implantação

```sh
./scripts/vagrant/deploy.sh
```

O `deploy.sh` copia os artefatos para `../fabric-network/chaincode/`
(montado em `/vagrant/chaincode` nas máquinas) e, em cada máquina:

1. instala o pacote CCaaS no peer;
2. aprova a definição `prontuarios` para a organização local;
3. registra o serviço systemd `prontuarios-cc`, que escuta em
   `127.0.0.1:9999` com `CHAINCODE_ID` igual ao *package ID*.

Por fim, confirma a definição no canal a partir de `fabric-node-1`.

### Atualização do chaincode

```sh
./scripts/vagrant/update.sh
```

No modelo CCaaS, o pacote registrado no canal contém apenas o endereço do
serviço, não o código. Uma mudança de código, portanto, não exige nova
aprovação nem nova sequência. O `update.sh`:

1. executa os testes unitários;
2. garante que as máquinas estão ligadas, que a rede privada funciona e que
   há líder Raft;
3. recompila e distribui os artefatos;
4. reaprova a sequência vigente apenas se o *package ID* mudou (rótulo ou
   porta);
5. prepara o binário nas três máquinas e reinicia os três serviços ao mesmo
   tempo, para reduzir o intervalo com versões diferentes;
6. confere, em cada máquina, o serviço, o hash do binário, o pacote aprovado
   e a versão respondida pelo chaincode (`getHeader`).

Os dados do ledger são preservados. Evite submeter transações durante a
atualização e prefira propriedades opcionais ao alterar tipos de ativos: os
ativos já gravados são revalidados quando atualizados.

Mudanças na **definição** (política de endosso, coleções privadas, versão da
definição) exigem uma nova sequência, aprovada por cada organização e
confirmada no canal, e não são automatizadas.

### Uso manual

```sh
vagrant ssh fabric-node-1
bash /vagrant/chaincode/check.sh
echo '{"key":{"@assetType":"institution","id":"2345678"}}' > /tmp/a.json
bash /vagrant/chaincode/cc.sh query readAsset /tmp/a.json
```

## Observações

- `build/pepper` (e sua cópia em `../fabric-network/chaincode/`) é o segredo
  do HMAC do CPF. Precisa ser idêntico em todos os peers e não deve ser
  versionado nem compartilhado fora do ambiente de testes. Trocar o segredo
  invalida a localização dos pacientes já cadastrados.
- Os profissionais não possuem identidade Fabric própria: a API de cada
  instituição assina com a identidade da organização, e o chaincode valida o
  vínculo do profissional com essa organização.
- A integração com IPFS, KMS e API (B-06, B-07 e B-09) não faz parte deste
  componente.
- Após retomar as máquinas de um estado salvo do VirtualBox, a rede
  *host-only* pode ficar parcialmente inacessível (Raft sem líder). Os
  scripts de `scripts/vagrant/` detectam a falha e restabelecem a
  comunicação reativando o enlace do adaptador
  (`VBoxManage controlvm <vm> setlinkstate2 off|on`) e limpando a tabela ARP
  (`sudo ip neigh flush dev eth1`).
