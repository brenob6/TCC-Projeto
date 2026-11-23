# Roteiro de demonstração

Roteiro para apresentar a prova de conceito à banca, seguindo a estrutura do
Capítulo 5 do TCC: requisitos (5.1), arquitetura (5.2), rede blockchain (5.3),
armazenamento distribuído (5.4), gerenciamento de chaves (5.5) e fluxos de
operação (5.6, Figuras 7 a 9).

Duração estimada: 25 minutos para o roteiro principal (cenas 0 a 9) e
10 minutos para as cenas complementares no terminal (cenas 10 a 12).

---

## Mapa: requisito, cena e evidência

| Requisito | Cena | O que a banca vê | Transação ou componente |
|---|---|---|---|
| RF-01 | 1 | Cadastro das instituições, cada uma assinada pela própria organização Fabric | `registerInstitution` |
| RF-02 | 1 | Cadastro de profissionais e de paciente; paciente sem nome nem CPF no ledger | `registerProfessional`, `registerPatient`, `getPatientByCpf` |
| RF-04 | 2, 8 | Concessão e revogação assinadas na carteira do paciente (Ed25519, *nonce*) | `grantPermission`, `revokePermission` |
| RF-03 | 3, 4, 9 | Escrita e leitura registradas na trilha de auditoria | `registerMedicalRecord`, `requestRecordAccess` |
| RF-07 | 3, 10 | Envelope cifrado no IPFS, disponível nos nós das três instituições | Kubo, rede privada com `swarm.key` |
| RF-05 | 4, 5, 11 | Leitura autorizada, leitura negada e decifração por limiar 2-de-3 | `requestRecordAccess`, `getAccessAuthorization`, custodiantes |
| RF-06 | 6 | Acesso emergencial com justificativa, de uso único | `requestEmergencyAccess`, `listEmergencyAccesses` |
| RF-09 | 7 | Eliminação lógica assinada pelo paciente e remoção do IPFS | `invalidateMedicalRecord`, `ipfs pin rm` |
| RF-08 | 9 | Histórico por paciente ou prontuário, com filtro de período | `getAccessHistory` |

---

## Preparação (antes da apresentação)

1. Suba o ambiente completo e aguarde o resumo final do script:

   ```sh
   ./start.sh
   ```

   São seis máquinas virtuais (cerca de 7,5 GB de RAM): `custodian-1..3`
   (`192.168.56.21-23`) e `fabric-node-1..3` (`192.168.56.31-33`), além da
   API Gateway no host.

   Confira, na etapa 2, as três linhas "IPFS troca blocos nos dois sentidos".
   Sem elas, a leitura de um prontuário por outra instituição (cenas 5 e 6)
   termina em `context deadline exceeded`; execute o `./start.sh` novamente.

2. Abra <http://127.0.0.1:8080> no navegador que será usado na apresentação.
   A chave de assinatura do paciente é gerada e guardada no `localStorage`
   desse navegador; em outro navegador, as operações assinadas ficam
   indisponíveis.

3. Deixe dois terminais abertos, um em `fabric-network/` e outro em
   `key-sharing/`, para as cenas complementares.

4. Ensaie o roteiro uma vez. O ledger é preservado entre execuções: se as
   instituições já tiverem sido cadastradas em um ensaio, a aba Instituição
   exibe os dados existentes e a cena 1 passa a ser apenas de apresentação.
   Para recomeçar do zero, é necessário recriar as máquinas.

5. Tenha à mão a Figura 6 (arquitetura) e as Figuras 7 a 9 (sequência), pois
   as respostas da interface exibem o `trace` com as mesmas etapas.

---

## Roteiro principal (interface web)

O seletor **Instituição em uso**, no topo da página, define a organização
Fabric com que a API assina as transações. Ele substitui, nesta prova de
conceito, a autenticação de usuários (Seção 5.3.3).

### Cena 0 — Arquitetura e disponibilidade (Seção 5.2, Figura 6, RF-07)

**Aba Rede.**

1. Apresente o cartão **Arquitetura** e relacione-o à Figura 6: três
   instituições, cada uma com orderer, peer, chaincode e nó IPFS na mesma
   máquina; três custodiantes de chaves em máquinas separadas.
2. Clique em **Verificar**. Mostre os três peers, os três nós IPFS e os
   custodiantes no esquema **2-de-3**.

**Mensagem para a banca:** a blockchain guarda apenas metadados e auditoria;
o conteúdo fica cifrado no IPFS e a chave é dividida entre custodiantes
independentes das instituições.

### Cena 1 — Cadastros e pseudonimização (Seção 5.3, RF-01, RF-02)

**Aba Instituição, com `Org1` selecionada.**

1. **Cadastrar instituição:** os campos já vêm preenchidos com o Hospital A
   (CNES `2345678`). Clique em **Cadastrar**. Destaque que cada organização
   Fabric representa uma única instituição.
2. **Profissionais de saúde:** cadastre uma médica (categoria médico,
   registro `CRM-DF 12345`). A habilitação para acesso extraordinário vem
   marcada por padrão para médicos.
3. **Cadastrar paciente:** informe um nome, mantenha o CPF gerado e clique em
   **Gerar par de chaves** antes de **Cadastrar paciente**. Explique:
   - o par Ed25519 é gerado no navegador, que faz o papel da carteira do
     paciente; a API recebe apenas a chave pública;
   - o KMS gera a chave de cifração e distribui os compartilhamentos aos
     custodiantes (Seção 5.5);
   - o CPF segue no mapa transiente da transação e não é gravado no bloco nem
     no estado; o nome fica fora da blockchain.
4. **Localizar paciente por CPF:** consulte o CPF usado acima. O chaincode
   recalcula o HMAC e encontra o paciente sem que o CPF esteja armazenado.

**Troque para `Org2`** e cadastre o Hospital B e um médico. **Opcional:** em
`Org3`, cadastre a Clínica C e um profissional não médico, sem habilitação
emergencial, para a variação da cena 6.

**Aba Paciente:** mostre os campos **Identificador pseudônimo**, **Hash do CPF
(HMAC)**, **Chave no KMS** e **Último nonce aceito** (0). É o que o ledger
conhece do paciente.

### Cena 2 — Consentimento assinado (Seção 5.3, RF-04)

**Aba Paciente, com `Org1` selecionada.**

1. Em **Conceder permissão**, escolha a médica do Hospital A como
   beneficiária, nível **leitura e escrita**, finalidade "acompanhamento
   clínico". Deixe escopo e validade em branco e comente que ambos existem
   para controle granular.
2. Clique em **Assinar e conceder**.
3. Em **Permissões concedidas**, mostre a permissão ativa. O **Último nonce
   aceito** passa a 1.

**Mensagem para a banca:** a mensagem assinada contém beneficiário, nível,
finalidade, escopo e validade. A instituição apenas encaminha a assinatura e
não consegue conceder, ampliar ou revogar permissões em nome do titular
(critério C3 do Quadro 1). A finalidade é obrigatória por exigência da LGPD.

### Cena 3 — Adição de prontuário (Seção 5.6, Figura 7, RF-03, RF-07)

**Aba Profissional, com `Org1` selecionada.**

1. Selecione a médica e o paciente. O indicador **Consentimento vigente**
   mostra leitura e escrita permitidas.
2. Em **Adicionar prontuário**, registre dois documentos, por exemplo uma
   consulta e um exame, com título e conteúdo.
3. Percorra o `trace` exibido e associe cada etapa à Figura 7:
   `checkPermission` → chave pública do paciente → cifração (ElGamal +
   AES-256-GCM) → `ipfs add` → `registerMedicalRecord` com CID e SHA-256 do
   envelope.

**Mensagem para a banca:** o registro exige permissão de leitura e escrita;
sem ela, a API recusa antes de gravar no IPFS e o chaincode rejeita a
transação.

### Cena 4 — Recuperação autorizada (Seção 5.6, Figura 8, RF-05)

**Mesma tela.**

1. Em **Prontuários acessíveis**, clique em **Abrir** no primeiro prontuário.
2. Mostre o conteúdo decifrado em **Resultado da leitura** e percorra o
   `trace`: `requestRecordAccess` (transação de escrita, auditada) →
   `getAccessAuthorization` → `ipfs cat` e verificação do SHA-256 →
   decifração parcial pelos custodiantes, cada um dos quais confirma no
   livro-razão o registro de auditoria e o SHA-256 do envelope.

**Mensagem para a banca:** a verificação de permissão é uma transação, e não
uma consulta, porque o RF-03 exige o registro de toda leitura (Quadro 4:
`hasPermission` corresponde a `requestRecordAccess` e
`getAccessAuthorization`).

### Cena 5 — Acesso negado e auditado (RF-05)

**Aba Profissional, com `Org2` selecionada.**

1. Selecione o médico do Hospital B e o mesmo paciente. O indicador mostra
   "sem leitura" e a lista de prontuários acessíveis está vazia.
2. Em **Recuperação extraordinária**, clique em **Solicitar leitura** no
   primeiro prontuário. O acesso é negado, e a mensagem informa que a
   negativa foi registrada na trilha de auditoria.

### Cena 6 — Recuperação extraordinária (Seção 5.6, Figura 9, RF-06)

**Mesma tela.**

1. Informe uma justificativa com pelo menos 10 caracteres, por exemplo
   "paciente inconsciente no pronto-socorro". O botão **Acesso emergencial**
   só é habilitado a partir desse tamanho.
2. Clique em **Acesso emergencial**. Mostre o conteúdo e o `trace`:
   `requestEmergencyAccess` seguido da mesma sequência da Figura 8; os
   custodiantes obtêm a justificativa no livro-razão.
3. Clique novamente em **Acesso emergencial** para o mesmo prontuário: a
   leitura é negada, pois a permissão extraordinária é de uso único
   (`removeExPermission` na Figura 9).

**Variação opcional:** em `Org3`, o profissional sem habilitação tem o acesso
emergencial recusado.

**Mensagem para a banca:** a permissão extraordinária vale para um único
prontuário, uma única leitura e uma hora. As permissões concedidas pelo
paciente não são alteradas, e o fluxo libera somente leitura.

### Cena 7 — Eliminação de prontuário (Seção 5.3.2, RF-09)

**Aba Paciente.**

1. Em **Acessos extraordinários**, mostre o acesso da cena 6, com
   profissional, instituição e justificativa, disponível para revisão pelo
   titular.
2. Em **Meus prontuários**, informe um motivo e clique em **Eliminar** no
   primeiro prontuário. A operação é assinada e o *nonce* avança.
3. Volte à aba **Profissional** com `Org1` e a médica: o prontuário eliminado
   deixa de ser listado. A API também removeu o envelope dos três nós IPFS.

**Mensagem para a banca:** como o ledger é imutável, a eliminação é lógica;
leituras posteriores são negadas. A trilha de auditoria permanece, pois não
contém dado clínico.

### Cena 8 — Revogação (RF-04)

**Aba Paciente.**

1. Em **Permissões concedidas**, clique em **Revogar** na permissão da médica.
2. Na aba **Profissional** (`Org1`, médica), o indicador passa a "sem leitura"
   e "sem escrita". Uma tentativa de leitura do segundo prontuário é negada e
   auditada.

### Cena 9 — Trilha de auditoria (Seção 5.3.1, RF-03, RF-08)

**Aba Auditoria.**

1. Consulte o histórico do paciente. A sequência deve conter, na ordem da
   demonstração: `CONCESSAO`, `CRIACAO` (duas), `LEITURA`, `ACESSO_NEGADO`,
   `SOLICITACAO_EMERGENCIAL`, `LEITURA_EMERGENCIAL`, `ACESSO_NEGADO`,
   `INVALIDACAO`, `REVOGACAO` e `ACESSO_NEGADO`.
2. Mostre, em cada registro, o responsável (profissional ou o próprio
   paciente), a instituição, a organização Fabric que assinou e o momento.
3. Filtre por prontuário e por período para atender literalmente ao RF-08.

**Mensagem para a banca:** os acessos negados também ficam registrados, o que
permite identificar tentativas de acesso indevido.

---

## Cenas complementares (terminal)

### Cena 10 — Rede IPFS privada (Seção 5.4, RF-07)

No terminal em `fabric-network/`:

```sh
vagrant ssh fabric-node-1 -c "ipfs swarm peers"
```

Devem aparecer apenas os outros dois nós, isolados da rede pública pela
`swarm.key`. Em seguida, com o CID do segundo prontuário (exibido no `trace`
da cena 3), mostre que o envelope está disponível a partir do nó de outra
instituição e que o conteúdo está cifrado:

```sh
vagrant ssh fabric-node-2 -c "ipfs cat <CID>"
```

A saída é o JSON com `c1`, `wrapped_key`, `nonce` e `ciphertext`, sem texto
clínico legível.

### Cena 11 — Custodiantes e tolerância a falhas (Seção 5.5, RF-05)

No terminal em `key-sharing/`, mostre que cada custodiante guarda apenas um
compartilhamento, com índice e valor distintos:

```sh
for node in custodian-1 custodian-2 custodian-3; do
  echo "[$node]"
  vagrant ssh "$node" -c "sudo sqlite3 -header -column /var/lib/custodian/shares.db \
    'SELECT key_id, share_index, created_at FROM shares'"
done
```

E a auditoria local de um custodiante, incluindo o acesso emergencial, sua
justificativa e o registro de auditoria do livro-razão que autorizou cada
decifração:

```sh
vagrant ssh custodian-1 -c "sudo sqlite3 -header -column /var/lib/custodian/shares.db \
  'SELECT key_id, requester_id, institution_id, emergency, reason, ledger_access_log, occurred_at
   FROM access_log ORDER BY id DESC LIMIT 5'"
```

Para mostrar que nem quem controla a API decifra sem autorização, solicite a
decifração diretamente aos custodiantes, com o certificado do coordenador e
sem registro de auditoria (substitua `<kmsKeyId>` pela chave do paciente,
exibida na aba Paciente, e `<cid>` pelo CID de um prontuário):

```sh
curl -s -X POST "http://192.168.56.31:5001/api/v0/cat?arg=<cid>" > /tmp/envelope.enc
./build/kms -custodians 192.168.56.21:50051,192.168.56.22:50051,192.168.56.23:50051 \
    -tls-ca build/tls/ca.crt -tls-cert build/tls/coordinator.crt -tls-key build/tls/coordinator.key \
    decrypt <kmsKeyId> /tmp/envelope.enc
```

A decifração é recusada pelos custodiantes. Com `-access-log` apontando para
um registro já utilizado pela API, a recusa também ocorre, pois cada registro
autoriza uma única decifração.

Para demonstrar o limiar, conceda novamente permissão de leitura à médica
(cena 2) e, então:

1. Pare um custodiante e leia o prontuário na interface: a leitura funciona
   com 2 de 3.

   ```sh
   vagrant ssh custodian-3 -c "sudo systemctl stop custodian"
   ```

2. Pare um segundo custodiante e tente ler: a decifração falha abaixo do
   limiar, e a aba **Rede** mostra os custodiantes indisponíveis. A tentativa
   continua registrada no ledger, pois a autorização ocorre antes da
   decifração.

   ```sh
   vagrant ssh custodian-2 -c "sudo systemctl stop custodian"
   ```

3. Restaure os custodiantes:

   ```sh
   vagrant ssh custodian-2 -c "sudo systemctl start custodian"
   vagrant ssh custodian-3 -c "sudo systemctl start custodian"
   ```

**Mensagem para a banca:** a chave privada nunca é reconstituída. Cada
custodiante calcula apenas uma decifração parcial, e o KMS combina as
parciais por interpolação de Lagrange no expoente.

### Cena 12 — Testes automatizados (B-10)

Mostre os testes de cada módulo e o roteiro de avaliação no ambiente integrado:

```sh
(cd prontuarios-cc/chaincode && go test ./...)
(cd api-gateway && go test ./...)
(cd key-sharing && go test ./...)
(cd frontend && npm test)
python3 avaliacao/medir.py resultados.json
```

O `medir.py` percorre os fluxos pela API Gateway, com conteúdo real cifrado,
e verifica, entre outros pontos, que o CPF não aparece no ativo do paciente e
que os custodiantes recusam decifrações sem autorização no livro-razão
(`avaliacao/README.md`).

---

## Limitações a apresentar

Convém mencioná-las antes que a banca pergunte, pois estão documentadas nos
READMEs dos módulos:

- **Solicitações de acesso pela API:** os custodiantes confirmam cada
  autorização no livro-razão, mas as transações de leitura e de acesso
  extraordinário são assinadas com a identidade da instituição, que fica na
  API. Uma API comprometida não decifra sem deixar registro, mas pode gerar
  registros legítimos; impedi-lo exigiria a assinatura do profissional.
- **Identidade:** as transações são assinadas com a identidade de cada
  organização; os profissionais não possuem certificado individual. A
  evolução natural é a emissão de certificados pelo Fabric CA.
- **Autenticação:** a interface não autentica usuários; a escolha da
  instituição e do profissional é feita na tela, para fins de demonstração.
- **Identidade dos custodiantes:** cada custodiante consulta o livro-razão com
  uma identidade emitida pela CA de uma organização, pois o chaincode restringe
  as consultas aos membros do consórcio; em produção, teriam um MSP próprio.
- **Diretório fora da blockchain:** a API mantém nomes e referências em
  `api-gateway/data/directory.json` para que a interface liste participantes;
  em um sistema real, esses dados ficariam no sistema de prontuário de cada
  instituição.
- **Segredo do HMAC:** compartilhado pelos peers do consórcio, o que exige uma
  decisão de governança sobre guarda e rotação.

---

## Encerramento

```sh
./stop.sh          # encerra a API; as máquinas continuam ligadas
./stop.sh --vms    # encerra também as máquinas, preservando os dados
```
