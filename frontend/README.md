# Frontend de demonstração

Interface React com TypeScript (Vite) para demonstrar os fluxos do TCC sobre a rede real,
por meio da API Gateway (`../api-gateway`).

## Telas

| Aba | Conteúdo | Requisitos |
|---|---|---|
| Rede | Arquitetura e disponibilidade de peers, nós IPFS e custodiantes | RF-07 |
| Instituição | Cadastro da instituição, de profissionais e de pacientes; consulta por CPF | RF-01, RF-02 |
| Profissional | Consentimento vigente, prontuários acessíveis, leitura (Figura 8), acesso extraordinário (Figura 9) e registro de prontuário (Figura 7) | RF-03, RF-05, RF-06 |
| Paciente | Carteira: concessão e revogação assinadas, eliminação de prontuário, acessos extraordinários | RF-04, RF-09 |
| Auditoria | Histórico por paciente ou prontuário, com filtro de período | RF-03, RF-08 |

O seletor **Instituição em uso**, no topo, define a organização Fabric com que
a API assina as transações.

Os fluxos das Figuras 7 a 9 exibem a sequência de interações da API com cada
camada (Blockchain, IPFS e KMS), com os identificadores das transações.

### Carteira do paciente

O par de chaves Ed25519 do paciente é gerado no navegador ao cadastrá-lo e
guardado no `localStorage`. As mensagens de concessão, revogação e eliminação
são montadas e assinadas em `src/wallet.ts`, no mesmo formato canônico de
`txdefs.SignedMessage`, e a API recebe apenas a assinatura. Em outro navegador,
as operações assinadas daquele paciente ficam indisponíveis.

## Execução

Com a API em `127.0.0.1:8080`:

```sh
npm install
npm run dev          # http://localhost:5173, com /api encaminhado à API
```

Para usar outra API: `API_URL=http://host:porta npm run dev`.

Build estático, servido pela própria API:

```sh
npm run build
cd ../api-gateway && go run ./cmd/api -static ../frontend/dist   # http://127.0.0.1:8080
```

## Roteiro de demonstração

1. **Rede:** confirme peers, nós IPFS e custodiantes disponíveis.
2. **Instituição:** com `Org1`, cadastre o Hospital A, uma médica e um
   paciente (gere o par de chaves antes). Repita com `Org2` para o Hospital B
   e um médico.
3. **Paciente:** conceda à médica do Hospital A permissão de leitura e escrita.
4. **Profissional (Org1):** registre um prontuário (Figura 7) e abra-o
   (Figura 8).
5. **Profissional (Org2):** tente ler o prontuário com o médico do Hospital B:
   o acesso é negado e auditado. Informe uma justificativa e use o acesso
   emergencial (Figura 9); uma segunda tentativa volta a ser negada (uso
   único).
6. **Paciente:** revogue a permissão e confira os acessos extraordinários;
   elimine o prontuário (RF-09).
7. **Auditoria:** consulte a sequência completa de operações.

## Estrutura

| Caminho | Conteúdo |
|---|---|
| `src/types.ts` | Tipos das respostas da API e dos ativos do chaincode |
| `src/api.ts` | Cliente tipado da API Gateway |
| `src/wallet.ts` | Carteira do paciente: chaves, mensagem canônica e assinatura |
| `src/components/ui.tsx` | Componentes comuns e exibição da sequência de interações |
| `src/views/` | Uma tela por aba |

## Verificação e testes

```sh
npm run typecheck    # tsc em modo estrito, sem emitir arquivos
npm test             # node --test, com remoção nativa de tipos do Node 24
```

O `npm run build` executa a verificação de tipos antes do build do Vite.

Cobrem a mensagem canônica (comparada ao exemplo do README do chaincode), o
formato de datas e a assinatura Ed25519. A compatibilidade byte a byte com o
Go foi conferida assinando a mesma mensagem com `cmd/patient-signer`.
