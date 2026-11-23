# Divergências entre a implementação do chaincode e o texto do TCC

Este documento lista os pontos em que a implementação dos itens B-02 a B-05
(`prontuarios-cc/`) difere da proposta descrita no `tcc.pdf` ou a detalha além
do que o texto afirma. Para cada item há a localização no texto, o que a
proposta diz, o que foi implementado e uma sugestão de redação.

As páginas seguem a numeração impressa no documento.

---

## 1. Organização dos contratos inteligentes

**Onde:** Seção 5.3 *Rede Blockchain* (p. 36–37); Seção 5.7, itens B-02 a B-05 (p. 40).

**Texto atual:** descreve cinco contratos (Registro de Usuários, Registro de
Prontuário Médico, Controle de Acesso, Recuperação de Prontuário Médico e
Recuperação Extraordinária), sugerindo chaincodes distintos.

**Implementação:** um único chaincode (`prontuarios`), construído com a
biblioteca cc-tools, com 7 tipos de ativos e 18 transações. Os cinco
contratos são grupos lógicos de transações. A separação em chaincodes
distintos exigiria chamadas entre chaincodes para cada verificação de
permissão, sem ganho de isolamento, pois todos operam no mesmo canal.

**Sugestão:** "Os contratos inteligentes foram implementados em um único
chaincode, desenvolvido com a biblioteca cc-tools, que organiza as regras de
negócio em tipos de ativos e transações. Os contratos descritos a seguir
correspondem a grupos de transações desse chaincode."

Convém incluir na Seção 4.2 uma subseção curta sobre o cc-tools, ao lado de
Hyperledger Fabric, gRPC e Vagrant.

---

## 2. Dados pessoais armazenados no cadastro de usuários

**Onde:** Seção 5.3, *Contrato de Registro de Usuários* (p. 36).

**Texto atual:** "Este contrato armazenará informações como nome, CPF e tipo
de usuário (paciente, médico, etc.)."

**Implementação:**
- Pacientes, profissionais e instituições são tipos de ativos distintos, e não
  um único "usuário" com campo de tipo.
- O ativo **paciente não armazena nome nem CPF**. O identificador é um UUID
  derivado de `HMAC-SHA256(segredo do consórcio, CPF)`, e o próprio HMAC é
  guardado para localizar o paciente.
- O CPF é enviado pelo **mapa transiente** da transação e, por isso, também
  não aparece no bloco. Isso foi verificado na rede: o bloco que registra o
  paciente não contém o CPF.
- O profissional guarda nome, categoria e registro no conselho, que são dados
  de exercício profissional, e o HMAC do CPF.

**Motivo:** o ledger é imutável e replicado entre as instituições. Gravar
nome e CPF em claro impediria a eliminação desses dados (LGPD, art. 18, VI) e
os exporia a todas as organizações, justamente a tensão discutida no
Capítulo 3 (Belen-Saglam et al., 2023). Com um HMAC baseado em segredo, o
CPF não pode ser recuperado por enumeração dos valores possíveis.

**Sugestão:** "O Contrato de Registro de Usuários mantém cadastros distintos
para instituições de saúde, pacientes e profissionais. Para o paciente, a
blockchain armazena apenas um identificador pseudônimo e um resumo HMAC-SHA256
do CPF, calculado com um segredo compartilhado pelo consórcio; o CPF é
transmitido em campo transiente da transação e não é persistido no livro-razão.
Dessa forma, nenhum dado pessoal do paciente é gravado de forma imutável."

**Impacto em outras seções:** a Seção 2.10 (LGPD) pode mencionar a
pseudonimização (LGPD, art. 13, § 4º) como técnica adotada, e a Seção 2.5
pode citar HMAC como uso de função de hash com chave.

---

## 3. Assinatura do paciente nos consentimentos

**Onde:** Seção 5.3, *Contrato de Controle de Acesso* (p. 36); RF-04 (p. 35);
Seção 5.5 *Gerenciamentos de chaves* (p. 37).

**Texto atual:** o paciente concede e revoga permissões, mas o texto não
descreve como o sistema comprova que a decisão partiu do paciente.

**Implementação:**
- Cada paciente possui um **par de chaves de assinatura Ed25519**, cuja chave
  pública é registrada no cadastro.
- A concessão (`grantPermission`), a revogação (`revokePermission`) e a
  eliminação de prontuário (`invalidateMedicalRecord`) só são aceitas com a
  assinatura do paciente sobre uma mensagem canônica que contém todos os
  campos da operação (beneficiário, nível, finalidade, escopo, validade).
- Um contador (*nonce*) sequencial impede a reutilização de uma assinatura.
- A assinatura fica registrada na permissão, como prova do consentimento.

**Motivo:** sem assinatura, a API da instituição poderia registrar
consentimentos em nome do paciente. A assinatura transfere ao titular o
controle efetivo sobre a concessão, coerente com o critério C3 do Quadro 1.

**Sugestão (5.3):** "Toda concessão ou revogação de permissão é assinada
digitalmente pelo paciente. O contrato verifica a assinatura sobre os dados da
operação e rejeita mensagens repetidas, de modo que nenhuma instituição
consegue conceder, ampliar ou revogar permissões em nome do titular."

**Sugestão (5.5):** o texto diz que "cada paciente possuirá uma chave
criptográfica". Passam a existir duas: a **chave de cifração**, gerada e
custodiada pelo KMS com Shamir Secret Sharing, e a **chave de assinatura**,
mantida pelo próprio paciente, por exemplo em aplicativo móvel. Vale explicar
a distinção e citar a Seção 2.6 (Assinatura digital).

---

## 4. Elementos do consentimento

**Onde:** Seção 5.3, *Contrato de Controle de Acesso* (p. 36); RF-04 e RF-05
(p. 35).

**Texto atual:** "gerenciar as permissões de acesso aos prontuários médicos".

**Implementação:** cada permissão registra:
- **beneficiário:** um profissional específico **ou** uma instituição inteira;
- **nível:** `leitura` ou `leituraEscrita`;
- **finalidade** obrigatória (a LGPD exige finalidade determinada, Seção 2.10);
- **escopo** opcional, restrito a prontuários específicos (somente leitura);
- **validade** opcional;
- situação (`ativa`, `revogada`, `expirada`), sendo a expiração avaliada no
  momento de cada acesso.

**Sugestão:** descrever esses atributos no contrato, destacando a finalidade
como requisito da LGPD e o escopo restrito como forma de controle granular.

---

## 5. Registro de prontuário exige consentimento

**Onde:** Seção 5.6, primeiro parágrafo, e Figura 7 (p. 38–39).

**Texto atual:** o médico solicita a adição e o hash é registrado,
"associando-o ao paciente", sem verificação de permissão.

**Implementação:** `registerMedicalRecord` exige uma permissão
`leituraEscrita` ativa do paciente para o autor ou para sua instituição. Sem
ela, a transação é rejeitada. O prontuário é associado ao paciente, ao autor,
à instituição de origem e ao identificador da chave no KMS.

**Consequência a registrar:** o fluxo extraordinário libera somente a
**leitura**. Um prontuário gerado durante o atendimento emergencial só poderá
ser registrado após o consentimento do paciente ou do responsável legal.

**Sugestão para a Figura 7:** incluir, antes de `registerCID`, a verificação
de permissão de escrita, e renomear a chamada para
`registerMedicalRecord(CID, contentHash, pID, mID, tipo)`.

---

## 6. Cálculo do hash do prontuário

**Onde:** Seção 5.3, *Contrato de Registro Prontuário Médico* (p. 36).

**Texto atual:** "Responsável por calcular o hash do prontuário médico
armazenado na rede IPFS e registrar esse hash na blockchain".

**Implementação:** o chaincode **não calcula** o hash, pois não tem acesso ao
arquivo, que não deve trafegar pela blockchain. A API calcula o SHA-256 do
envelope **cifrado** e o envia junto com o CID devolvido pelo IPFS; o contrato
valida o formato e registra ambos. O CID já é um identificador derivado do
conteúdo, e o SHA-256 permite verificar a integridade do arquivo devolvido
pelo IPFS.

**Sugestão:** "Responsável por registrar na blockchain o CID do prontuário
cifrado armazenado na rede IPFS e o resumo SHA-256 do envelope cifrado,
calculado pela API, associando-os ao paciente, ao profissional responsável e
à instituição de origem."

---

## 7. Verificação de permissão como transação auditada

**Onde:** Seção 5.6, segundo parágrafo, e Figura 8 (p. 38–39); RF-03 (p. 35).

**Texto atual:** na Figura 8, o KMS chama `hasPermission(mID, pID)` na
blockchain, que responde `true`.

**Implementação:**
1. A API, com a identidade da organização da instituição, submete
   `requestRecordAccess(CID, mID)`. É uma **transação de escrita**: decide o
   acesso e grava um registro de auditoria (`LEITURA` ou `ACESSO_NEGADO`).
   Devolve `granted`, o identificador do registro de auditoria e o `kmsKeyId`.
2. A API envia ao KMS o envelope cifrado e o identificador do registro.
3. Antes de decifrar, o KMS consulta `getAccessAuthorization`, que confirma
   que o registro corresponde a uma leitura autorizada, recente (padrão:
   5 minutos) e relativa à mesma chave.

**Motivos:**
- Uma consulta (`query`) não gera registro no livro-razão. Como RF-03 exige o
  registro das leituras, a verificação precisa ser uma transação confirmada.
- O KMS não é membro da rede Fabric; quem possui identidade é a organização de
  cada instituição.
- **Acessos negados também ficam registrados**, o que reforça RF-08 e não
  estava previsto no texto.

**Sugestão para a Figura 8:**
`API → Blockchain: requestRecordAccess(CID, mID)` → `{granted, accessLogId, kmsKeyId}`;
`API → IPFS: getDocument(CID)`;
`API → KMS: decrypt(documento, accessLogId)`;
`KMS → Blockchain: getAccessAuthorization(accessLogId)` → `valid`.

**Observação:** a validação no KMS (B-07/B-09) ainda não foi integrada; o
README de `key-sharing/` já registra a vinculação como trabalho pendente.

---

## 8. Recuperação extraordinária

**Onde:** Seção 5.3, *Contrato de Recuperação Extraordinária* (p. 36–37);
Seção 5.6, terceiro parágrafo, e Figura 9 (p. 39–40); RF-06 (p. 35).

**Texto atual:** profissionais "autorizados" acessam sem permissão prévia; a
blockchain salva o motivo e, após a recuperação, "revoga a permissão de
acesso ao prontuário".

**Implementação:**
- `requestEmergencyAccess(pID, mID, CID, justificativa)` cria uma permissão
  extraordinária **restrita a um prontuário**, **de uso único** e válida por
  **1 hora**.
- "Profissional autorizado" passou a ter definição: profissional ativo,
  vinculado à instituição que submete a transação e com a habilitação
  `emergencyAccessAllowed` (por padrão, médicos).
- A justificativa é obrigatória (mínimo de 10 caracteres) e fica registrada
  na permissão e na trilha de auditoria.
- A leitura ocorre em `requestRecordAccess`, que registra
  `LEITURA_EMERGENCIAL` e marca a permissão como `utilizada`, o
  `removeExPermission` da Figura 9. Uma segunda leitura é negada.
- O que é revogado é **apenas a permissão extraordinária**; as permissões
  concedidas pelo paciente não são alteradas. O texto atual pode sugerir o
  contrário.
- `listEmergencyAccesses` permite a revisão posterior pelo paciente e pelas
  instituições.

**Sugestão (5.6):** "Após a leitura, a permissão extraordinária é consumida e
deixa de ser válida, de modo que cada solicitação emergencial autoriza uma
única leitura de um único prontuário, dentro de uma janela de uma hora. As
permissões concedidas pelo paciente permanecem inalteradas."

**Sugestão para a Figura 9:** a chamada `exRecover(mID, pID, description)`
passa a incluir o CID e corresponde a `requestEmergencyAccess`; a verificação
`hasPermission` corresponde a `requestRecordAccess`, como no item 7.

---

## 9. Eliminação lógica de prontuário (novo)

**Onde:** não há correspondente; relaciona-se à Seção 2.10, ao Capítulo 3
(direito ao esquecimento) e ao objetivo específico sobre LGPD (p. 13).

**Implementação:** `invalidateMedicalRecord`, assinada pelo paciente, marca o
prontuário como eliminado. Leituras posteriores são negadas e o prontuário
deixa de ser listado. A remoção do arquivo no IPFS (*unpin*) cabe à API.

**Sugestão:** acrescentar um requisito, por exemplo "RF-09 – O sistema deve
permitir que o paciente solicite a eliminação de seus prontuários", e
descrever a eliminação lógica no contrato de Registro de Prontuário. Vale
registrar a limitação: o histórico de auditoria permanece, pois não contém
dado clínico.

---

## 10. Trilha de auditoria

**Onde:** RF-03 e RF-08 (p. 35); Seção 5.3 (p. 36).

**Texto atual:** registro de leituras e escritas e consulta do histórico com
usuário, instituição, operação e momento.

**Implementação:** ativo próprio (`accessLog`), somente de inserção, com as
operações `CRIACAO`, `LEITURA`, `LEITURA_EMERGENCIAL`,
`SOLICITACAO_EMERGENCIAL`, `CONCESSAO`, `REVOGACAO`, `INVALIDACAO` e
`ACESSO_NEGADO`. Cada registro identifica o profissional responsável (ou o
próprio paciente, nas operações assinadas por ele), a instituição, a
organização Fabric que assinou, a permissão ou o acesso extraordinário que
fundamentou a operação e o momento. `getAccessHistory` atende RF-08, por
paciente ou por prontuário e com filtro de período.

**Sugestão:** listar as operações auditadas e mencionar que as negativas
também são registradas.

---

## 11. Modelo de identidade e "API Gateway"

**Onde:** Figura 6 (p. 36) e Figuras 7 a 9.

**Texto atual:** uma única "API Gateway" intermedeia todas as chamadas.

**Implementação:** cada instituição é uma organização Fabric (`Org1MSP` a
`Org3MSP`) e as transações são assinadas com a identidade dessa organização.
O chaincode verifica que o profissional informado pertence à instituição do
chamador; uma organização não consegue agir em nome de profissionais de outra.
Os profissionais não possuem identidade Fabric individual nesta prova de
conceito.

**Sugestão:** explicitar que há uma instância da API por instituição, ou uma
API que opera com a identidade de cada organização, e registrar como
limitação a ausência de identidades individuais. Uma evolução possível é a
emissão de certificados por profissional com Fabric CA.

---

## 12. Infraestrutura do chaincode

**Onde:** Seção 4.2.1 (p. 31–32) e Seção 5.3 (p. 36).

**Implementação:**
- O chaincode é executado como **serviço externo (CCaaS)**, um serviço
  systemd em cada máquina, sem Docker.
- O banco de estado é **LevelDB**. As listagens usam índices de chave
  composta, sem consultas ricas do CouchDB.
- A política de endosso é **maioria das três organizações**.
- Há **um único canal** (`prontuarios`) e não são usadas *private data
  collections*. A Seção 5.3 cita "suporte a canais privados" como motivação;
  convém deixar claro que o recurso não foi necessário, porque nenhum dado
  pessoal ou clínico é gravado no ledger (itens 2 e 6).
- O segredo do HMAC do CPF é compartilhado pelos peers do consórcio, o que
  implica uma decisão de governança sobre sua guarda e rotação.

---

## 13. Correspondência entre os nomes dos diagramas e as transações

| Diagrama | Chamada no texto | Transação implementada |
|---|---|---|
| Figura 7 | `registerCID(CID, pID)` | `registerMedicalRecord` |
| Figura 8 | `hasPermission(mID, pID)` | `requestRecordAccess` (API) e `getAccessAuthorization` (KMS) |
| Figura 9 | `exRecover(mID, pID, description)` | `requestEmergencyAccess` |
| Figura 9 | `removeExPermission` | consumo da permissão dentro de `requestRecordAccess` |
| — | consentimento (RF-04) | `grantPermission`, `revokePermission` |
| — | histórico (RF-08) | `getAccessHistory` |

---

## 14. Cronograma de implementação

**Onde:** Seção 5.7, Quadros 4 e 5 (p. 40–41).

**Texto atual:** B-02 a B-05 aparecem como entregas sequenciais, entre 25/08
e 12/10.

**Implementação:** B-02 a B-05 foram entregues em conjunto, como um único
chaincode. Se o texto final relatar o cronograma executado, os Quadros 4 e 5
devem refletir as datas reais. Este documento não as registra.

---

## 15. Resumo e Conclusão

**Onde:** Resumo/Abstract e Capítulo 6 (p. 42).

**Sugestão:** acrescentar, onde a arquitetura é resumida, que o paciente
assina digitalmente seus consentimentos, que nenhum dado pessoal ou clínico é
gravado na blockchain (pseudonimização do paciente) e que todas as leituras,
inclusive as negadas e as emergenciais, ficam registradas de forma imutável.
