# API Gateway

Componente *API Gateway* da arquitetura (Figura 6 do TCC): recebe as
requisições do frontend (`../frontend`) e coordena as três camadas.

| Camada | Componente | Acesso |
|---|---|---|
| Blockchain | `../fabric-network` + `../prontuarios-cc` | Fabric Gateway (`fabric-gateway` v1.10), peer de cada organização |
| Armazenamento | `../ipfs-private` (nas máquinas de `../fabric-network`) | API HTTP do Kubo (porta 5001) do nó de cada instituição |
| Chaves | `../key-sharing` | Cliente do KMS (`kmsclient`) falando gRPC com os custodiantes |

A API opera com a identidade de **cada organização** (Seção 5.3.3): o
cabeçalho `X-Org` (`Org1`, `Org2`, `Org3`) define com qual certificado a
transação é assinada e quais peer e nó IPFS são usados. O chaincode verifica
que o profissional informado pertence à instituição dessa organização.

## Fluxos

| Figura | Rota | Sequência |
|---|---|---|
| 7 | `POST /api/records` | `checkPermission` → chave pública do paciente → cifração (ElGamal + AES-256-GCM) → `ipfs add` → `registerMedicalRecord` |
| 8 | `POST /api/records/{cid}/access` | `requestRecordAccess` (auditado) → `getAccessAuthorization` → `ipfs cat` + SHA-256 → decifração pelos custodiantes, que confirmam o `accessLog` e o SHA-256 do envelope no livro-razão |
| 9 | `POST /api/records/{cid}/emergency` | `requestEmergencyAccess` → mesma sequência da Figura 8; os custodiantes obtêm a justificativa no livro-razão |

As respostas desses fluxos trazem o campo `trace`, com cada interação entre a
API e as camadas, exibido pelo frontend.

Demais rotas:

| Rota | Transação |
|---|---|
| `GET /api/health` | `getHeader` em cada peer, `ipfs id` em cada nó, `Health` em cada custodiante |
| `GET /api/directory` | Organizações, profissionais e pacientes conhecidos pela API |
| `GET /api/assets/{key}` | `readAsset` |
| `POST /api/institutions` · `PUT /api/institutions/status` | `registerInstitution` · `setInstitutionStatus` |
| `POST /api/professionals` · `PUT /api/professionals/{key}/status` | `registerProfessional` · `setProfessionalStatus` |
| `POST /api/patients` | `kms keygen` + `registerPatient` |
| `POST /api/patients/lookup` | `getPatientByCpf` |
| `GET /api/patients/{key}/records?professional=` | `listPatientRecords` |
| `GET /api/patients/{key}/all-records` | Diretório da API + `readAsset` de cada prontuário |
| `GET/POST /api/patients/{key}/permissions` | `listPermissions` · `grantPermission` |
| `GET /api/patients/{key}/check?professional=` | `checkPermission` |
| `GET /api/patients/{key}/history?record=&from=&to=` | `getAccessHistory` |
| `GET /api/patients/{key}/emergencies` | `listEmergencyAccesses` |
| `POST /api/permissions/{key}/revoke` | `revokePermission` |
| `POST /api/records/{cid}/invalidate` | `invalidateMedicalRecord` + `ipfs pin rm` nos três nós |

`{key}` é a chave do cc-tools (`patient:<uuid>`, `professional:<uuid>`, ...).

### Assinatura do paciente

Concessão, revogação e eliminação chegam à API já assinadas pela carteira do
paciente (Ed25519, gerada no navegador). A API apenas encaminha `nonce` e
`signature`; sem a chave privada, não consegue forjar nem alterar um
consentimento.

### Diretório fora da blockchain

O ledger não oferece listagens gerais nem guarda o nome do paciente. Para que
a interface liste participantes, a API mantém `data/directory.json` com
instituições, profissionais, nomes de pacientes e a referência dos prontuários
que registrou. O CPF não é gravado. Em um sistema real, esses dados ficariam no
sistema de prontuário de cada instituição.

## Pré-requisitos

- Go 1.24
- Rede Fabric criada, com os nós IPFS, e chaincode implantado
  (`../fabric-network/scripts/start.sh` e
  `../prontuarios-cc/scripts/vagrant/deploy.sh`); a API usa o material
  criptográfico de `../fabric-network/organizations`
- Custodiantes de chaves no modo `livro-razao`, com o certificado do
  coordenador em `../key-sharing/build/tls/` (`../key-sharing/scripts/gen-tls.sh`
  e `../key-sharing/scripts/ledger-config.sh`, executados pelo `start.sh` da raiz)

O `start.sh` da raiz do repositório prepara todo o ambiente e inicia a API.

## Execução

```sh
go run ./cmd/api                                  # http://127.0.0.1:8080
go run ./cmd/api -static ../frontend/dist         # serve também o frontend compilado
```

| Opção | Padrão | Descrição |
|---|---|---|
| `-config` | `config.json` | Arquivo opcional que sobrepõe os valores padrão (ver `config.example.json`) |
| `-organizations` | `../fabric-network/organizations` | Material criptográfico das organizações |
| `-listen` | `127.0.0.1:8080` | Endereço HTTP |
| `-static` | vazio | Build do frontend, servido em `/` |

Os valores padrão correspondem à topologia Vagrant: peers em
`192.168.56.31-33:7051` (identidade `User1@orgN`), nós IPFS nas mesmas
máquinas, em `192.168.56.31-33:5001`, e custodiantes em `192.168.56.21-23:50051`, limiar 2-de-3,
com TLS mútuo (`custodianTls`: CA e certificado do coordenador em `../key-sharing/build/tls/`).

## Testes

```sh
go test ./...
```

Os testes usam dublês do ledger, do IPFS e do KMS e cobrem: recusa de escrita
sem consentimento antes de gravar no IPFS, registro e leitura, leitura negada
sem acionar IPFS e KMS, rejeição de envelope adulterado, remoção do envelope
após recusa do chaincode, encaminhamento dos campos assinados, cadastro de
paciente com geração de chave no KMS, tradução dos erros do chaincode, envio
do `accessLog` e do envelope recuperado do IPFS ao KMS e resposta 403 quando os
custodiantes recusam a decifração.

## Limitações da prova de conceito

- A API detém as identidades das instituições (`User1@orgN`) e pode, se
  comprometida, solicitar leituras e acessos extraordinários em nome dos
  profissionais. Os custodiantes recusam decifrações sem registro no
  livro-razão, de modo que esse abuso fica registrado, mas não é impedido;
  impedi-lo exigiria a assinatura do profissional na solicitação de acesso.
- A API tem acesso ao conteúdo decifrado, pois é ela quem o entrega ao
  profissional.
- A API não autentica o usuário do frontend: a escolha da instituição e do
  profissional é feita na interface, para fins de demonstração.
