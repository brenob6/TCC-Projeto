# Avaliação

`medir.py` percorre os fluxos de operação da prova de conceito pela API
Gateway, verifica o resultado de cada etapa e mede a latência de cada
requisição. É o roteiro usado na avaliação descrita no capítulo de
Resultados e Discussão do TCC.

## Execução

```sh
./start.sh                               # ambiente completo em execução
python3 avaliacao/medir.py resultados.json
```

Requisitos: Python 3 (somente a biblioteca padrão), os binários
`prontuarios-cc/build/patient-signer` e `key-sharing/build/kms`, gerados
durante o `start.sh`, o certificado do coordenador em
`key-sharing/build/tls/`, e o Vagrant, usado para parar e iniciar
custodiantes e para ler o envelope em outro nó IPFS.

| Variável | Padrão | Efeito |
|---|---|---|
| `API` | `http://127.0.0.1:8080` | Endereço da API Gateway |
| `REPS` | `5` | Repetições da adição e da leitura para cada tamanho de documento |
| `OP_REPS` | `5` | Repetições das demais operações (cadastros, consentimentos, eliminação, leitura com 2 de 3 custodiantes e histórico) |

## Etapas

1. Disponibilidade da API, dos peers, dos nós IPFS e dos custodiantes.
2. Cadastro de dois profissionais (Org1 e Org2) e de um paciente com um par
   de chaves Ed25519 novo; verifica que o CPF não aparece no ativo e que o
   paciente é localizado pelo CPF a partir de outra organização.
3. Concessão de permissão assinada pelo paciente; verifica a recusa da
   repetição da assinatura e de uma assinatura sobre campos alterados.
4. Adição e leitura autorizada de documentos de 1 KiB, 10 KiB, 100 KiB e
   1 MiB.
5. Leitura do envelope a partir do nó IPFS de outra instituição; verifica
   que ele não contém o texto em claro.
6. Leitura sem permissão (negada e auditada).
7. Decifração solicitada diretamente aos custodiantes, sem passar pela API,
   com o certificado do coordenador (simulação de uma API comprometida):
   verifica a recusa sem registro de auditoria, com o registro de uma leitura
   negada, com um registro inexistente, com um registro já utilizado e com um
   registro válido acompanhado do envelope de outro prontuário.
8. Acesso extraordinário com justificativa; verifica o uso único e a recusa
   de justificativa curta.
9. Tolerância a falhas: `OP_REPS` leituras com 2 de 3 custodiantes (sucesso) e
   uma com 1 de 3 (falha na decifração), seguidas da restauração dos
   custodiantes.
10. Eliminação de prontuário e revogação de permissão, ambas assinadas pelo
    paciente, com verificação da negação das leituras seguintes.
11. Repetição, `OP_REPS` vezes, das operações executadas uma única vez no
    fluxo principal: cadastro de profissional e de paciente, localização pelo
    CPF, concessão, eliminação de um prontuário de 1 KiB e revogação, cada
    repetição com um paciente novo.
12. Consulta do histórico de acessos a partir de Org3, `OP_REPS` vezes.

## Saída

O arquivo JSON contém o resultado de cada verificação (`checks`), as
latências de cada requisição (`latency`) e o resumo por operação
(`summary`: amostras, média, mediana, mínimo, máximo e desvio-padrão), o
tamanho dos envelopes (`envelope`), a saída de cada tentativa de decifração
direta (`direct_decrypt`) e a contagem de operações do histórico
(`history_ops`). O script termina com código 1 se alguma verificação falhar.

## Efeitos colaterais

- Os cadastros, prontuários e registros de auditoria criados permanecem no
  ledger e em `api-gateway/data/directory.json`, pois o ledger não admite
  remoção.
- Os serviços `custodian` de `custodian-2` e `custodian-3` são parados
  durante a etapa 9 e reiniciados ao final dela.
