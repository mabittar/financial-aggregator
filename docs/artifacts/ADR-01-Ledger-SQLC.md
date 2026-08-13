# ADR-01-Ledger-SQLC: Arquitetura, Segurança e Otimização de Performance com SQLC e PostgreSQL em Go

## Status: Proposta

## Resumo

Este documento descreve a adoção do **SQLC** como ferramenta de geração de código Go para acesso a banco de dados PostgreSQL em toda a aplicação financiária do Ledger. O SQLC é utilizado em conjunto com o driver `pgx/v5` para garantir **tipagem segura**, **performance de I/O otimizada** e **integridade transacional**. Este ADR define os princípios de configuração, padrões de consulta, mitigação de segurança, dimensionamento de pool de conexões e arquitetura aplicável ao DDD.

---

## 1. Fundamentos de Arquitetura e Configuração Avançada do SQLC

### 1.1. Otimização do Ficheiro de Configuração

O ficheiro `sqlc.yaml` (versão 2) é o ponto de entrada para a geração de código. Uma configuração inadequada pode resultar em alocações de memória desnecessárias, dificuldades na serialização de dados e código excessivamente acoplado.

A tabela seguinte descreve as opções de configuração críticas para a geração de código Go robusto:

| Opção de Configuração (sqlc.yaml) | Valor Recomendado | Mecanismo e Justificação Técnica |
|---|---|---|
| `sql_package` | `"pgx/v5"` | Delega a gestão de tipos e protocolos ao driver especializado pgx. Habilita o uso de binários do PostgreSQL, contornando a lenta serialização textual exigida pelas interfaces `database/sql` genéricas. |
| `emit_pointers_for_null_types` | `true` | Historicamente, colunas anuláveis eram mapeadas para tipos como `sql.NullString` ou `pgtype.Text`. Com esta flag, o SQLC gera ponteiros nativos do Go (`*string`). Simplifica a serialização JSON e a manipulação na camada de domínio. |
| `emit_interface` | `true` | Gera a interface `Querier` com as assinaturas de todos os métodos de acesso aos dados. Esta abstração é o pilar da testabilidade, permitindo a geração de mocks (ex.: via `mockgen`) para isolar a base de dados durante testes unitários. |
| `emit_empty_slice` | `true` | Força a inicialização de slices vazias (`[]`) em consultas `:many` que não retornem resultados, em vez de retornar o valor `nil`. Previne vulnerabilidades de ponteiros nulos (Nil Pointer Dereference) e formatagem adequada de arrays vazios em respostas de APIs REST. |
| `emit_params_struct_pointer` | `true` | Altera a assinatura das funções geradas para aceitar os parâmetros como ponteiros `(*Params)`, reduzindo a sobrecarga de alocação e cópia de memória por valor em métodos com dezenas de argumentos ou payloads avolumados. |
| `emit_result_struct_pointer` | `true` | Instrução homóloga à anterior, mas focada nos resultados. Garante que as funções devolvem ponteiros para os modelos `(*Row)`, beneficiando o ciclo de Garbage Collection (GC) do Go ao gerir grandes volumes de leitura no heap. |
| `omit_unused_struct` | `true` | Analisa a topologia das consultas e inibe a geração de modelos (structs) Go referentes a tabelas ou ENUMs que constem no esquema DDL, mas que não sejam acedidos por qualquer instrução SQL, mantendo a base de código limpa. |

### 1.2. Mapeamento Semântico e Substituição de Tipos (Type Overrides)

A heurística de mapeamento do SQLC é sofisticada, mas o desenho de sistemas complexos requer uma tipagem de domínio precisa. O mecanismo de overrides possibilita a substituição das escolhas padrão do compilador.

#### UUID

O PostgreSQL dispõe do tipo `uuid`, que o SQLC traduz, por defeito, para `pgtype.UUID`. A comunidade Go padronizou o pacote `github.com/google/uuid` para a manipulação destas entidades, dada a sua interoperabilidade com sistemas de routing web e serialização.

**Configuração do override:**

```yaml
# sqlc.yaml
sql_package: "pgx/v5"
emit_pointers_for_null_types: true

# Tipos de override podem ser adicionados aqui
overrides:
  - name: "uuid"
    type: "github.com/google/uuid.UUID"
    # Mapeia a coluna uuid do banco para o tipo google/uuid.UUID
```

#### JSON e JSONB

Por omissão, uma coluna `jsonb` resulta num simples `[]byte` em Go. Quando o esquema do JSON é conhecido e rígido, é imperativo utilizar a configuração de overrides baseada em colunas (coluna: `"tabela.coluna"`) para instruir o SQLC a associar essa coluna específica a uma estrutura Go definida na aplicação (ex.: `domínio.MetadadosTransacao`). O `pgx/v5` assumirá o processo de marshal e unmarshal binário automaticamente.

#### Timezones (timestamptz)

Os timezones (tipo `timestamptz`) devem ser mapeados explicitamente para o tipo nativo `time.Time`, assegurando que o processamento temporal não utiliza abstrações intermediárias, aproveitando a biblioteca de precisão do Go.

---

## 2. Padrões Avançados de Consultas e Limitações Estruturais

### 2.1. Nulidade e a Evolução para a Sintaxe `@param`

A gestão de filtros dinâmicos é um ponto onde os utilizadores de ORMs sentem mais atrito ao transitar para o SQL bruto. Num ORM, a consulta é montada em tempo de execução consoante a existência de parâmetros. No SQLC, o SQL deve ser estático.

#### Abordagem Clássica com `sqlc.narg` (Antiga)

```sql
-- Abordagem clássica com sqlc.narg
SELECT * FROM utilizadores 
WHERE (sqlc.narg('nome')::text IS NULL OR nome = sqlc.narg('nome'))
```

Este padrão forçava casts explícitos (`::text`) no PostgreSQL para ajudar o analisador estático do SQLC a inferir os tipos corretos, degradando a legibilidade.

#### Abordagem Moderna com Anotação de Parâmetros (`@param`)

```sql
-- Abordagem moderna com anotação de parâmetros
-- @param nome? TEXT
SELECT * FROM utilizadores 
WHERE (@nome IS NULL OR nome = @nome)
```

A nova sintaxe limpa a declaração SQL e clarifica a precedência das diretivas de nulidade na geração do código. A utilização deste padrão dinâmico de pesquisa tem profundas repercussões nos planos de execução da base de dados, um tema abordado na secção de performance.

### 2.2. A Falsidade do Macro `sqlc.slice` em PostgreSQL

O SQLC faculta a macro `sqlc.slice()` para motores que não suportam matrizes, como o MySQL e o SQLite. Esta macro reescreve a consulta em tempo de execução, expandindo dinamicamente a quantidade de placeholders consoante a dimensão da fatia passada (slice).

**A expansão dinâmica da consulta invalida o uso de Prepared Statements:**

- Cada nova dimensão do array resulta numa assinatura de consulta distinta.
- Inflaciona a memória de cache.
- Exige um novo parsing contínuo no motor da base de dados.

Em ambientes PostgreSQL configurados com o driver `pgx/v5`, a utilização de `sqlc.slice` é classificada como um anti-pattern rigoroso. O PostgreSQL suporta tipos primitivos de Arrays.

**Prática recomendada em PostgreSQL (sem quebra do Prepared Statement):**

```sql
-- Prática recomendada em PostgreSQL (sem quebra do Prepared Statement)
SELECT * FROM produtos
WHERE categoria_id = ANY($1::INT[]);
```

O `pgx` serializa o slice de Go (`[]int32`) de forma binária e transparente para o formato de array do Postgres, mantendo o plano de execução intocável e maximizando a taxa de transferência de rede (throughput).

### 2.3. A Armadilha de Estruturas Aninhadas (`sqlc.embed`) com LEFT JOIN

O `sqlc.embed` permite reaproveitar estruturas de modelos nas respostas das consultas, injetando uma tabela inteira como uma propriedade aglomerada em vez de listar dezenas de colunas horizontalmente.

**Numa junção interna (INNER JOIN):** Este mecanismo funciona com graciosidade e resolve o problema dos mapeamentos exaustivos em Go.

**Numa junção externa (LEFT JOIN):** Introduz uma falha estrutural gravosa conhecida como a armadilha do Erro de Leitura Nula (Null Scan Error).

Quando o PostgreSQL resolve um LEFT JOIN sem correspondência, preenche todas as colunas da tabela secundária com NULL. O código Go gerado pela macro tenta injetar (scan) esses nulos na estrutura embutida da tabela. Se as definições DDL originais dessa tabela secundária estabelecerem que certas colunas são do tipo `NOT NULL`, o Go utilizará os tipos não mapeados a ponteiros (como `int64` puro). Consequentemente, a tentativa do driver de ler um NULL proveniente da junção não satisfeita para uma variável primitiva resulta num pânico inevitável em tempo de execução.

**Soluções arquiteturais para mitigar este defeito:**

1. **Agregação JSON no Servidor:** Delegar a resolução estrutural ao motor do banco de dados utilizando funções como `json_build_object` ou `json_agg`, empacotando os resultados da junção externa num único campo JSONB. O sistema de mapeamento JSON do Go tolera harmoniosamente a nulidade se o objeto não existir.

2. **Expansão Manual Segura:** Selecionar os atributos explicitamente e aplicar a cláusula protetora `COALESCE` para definir valores sentinela padronizados nos campos críticos quando ocorrer a omissão do LEFT JOIN.

3. **Encapsulamento através de Views:** Criar uma abstração lógica (View) no esquema do PostgreSQL que represente a junção de forma tolerante a falhas. O analisador DDL inferirá a nulidade correta da projeção e garantirá a geração de código Go com o mapeamento e ponteiros correspondentes.

---

## 3. Segurança e Integridade Transacional

### 3.1. Proteção Base contra SQL Injection e Mecanismos do Go

O risco fundamental da linguagem estruturada de pesquisa, a injeção de SQL, encontra-se arquiteturalmente selado pelo SQLC. Dado que todo o processamento de variáveis é efetuado através do Bind Protocol do PostgreSQL com marcadores posicionais (`$1, $2`), as variáveis do runtime de Go nunca são concatenadas sintaticamente às sentenças em formato de string. O interpretador garante que todos os dados anexados são tratados estritamente como literais (ou escalares) não executáveis.

A postura de segurança deve expandir-se para a camada concorrente e de lógica da aplicação Go. O pacote de ferramentas nativas deve ser englobado sistematicamente nas pipelines de auditoria de código:

- **Gestão de Vulnerabilidades (`govulncheck`):** As atualizações recorrentes dos drivers (como o `pgx`) podem introduzir falhas de segurança CVE. O comando `govulncheck` executa uma análise estática suportada pela base de dados de vulnerabilidades da equipa central da linguagem Go, identificando se caminhos da árvore de código atingem dependências comprometidas.

- **Testes Aleatorizados Estocásticos (Fuzzing):** Como o acesso aos dados transaciona com as margens extremas do domínio da aplicação, é fundamental testar as funções geradas através da infraestrutura de Fuzzing do Go. O fuzzer injeta massivamente dados impensáveis, strings Unicode mal formadas e limites inteiros para revelar edge-cases no parser da API, prevenindo ataques de denegação de serviço (DoS) provocados por pânicos nas abstrações de rede.

- **Detector de Condições de Corrida (Race Detector):** Os processos assíncronos (goroutines) são largamente utilizados para paralelizar chamadas à base de dados. Quando múltiplas goroutines transacionam sobre memória partilhada não sincronizada, gera-se uma condição de corrida. A compilação da suíte de testes com a flag `-race` (`go test -race ./...`) expõe e reporta acessos concorrentes imprevistos na memória antes da implantação.

### 3.2. Análise Estática de Regras via `sqlc vet`

O comando `sqlc vet` é um validador semântico contínuo. Ele varre o repositório de consultas e confronta cada ficheiro com um conjunto de regras heurísticas e corporativas implementadas em Common Expression Language (CEL).

O grande trunfo do `sqlc vet` manifesta-se quando acoplado à regra interna `sqlc/db-prepare` num ambiente com acesso a uma base de dados espelho ou contentorizada no processo de CI/CD. Nessas condições, a ferramenta prepara a consulta no banco de dados e recolhe o resultado prático da função `EXPLAIN`, preenchendo as varáveis analíticas globais postgresql.explain com a simulação do plano de execução real da base de dados.

**Tabela de regras de bloqueio críticas:**

| Nome da Regra de Segurança | Finalidade do Bloqueio da Submissão (Commit) | Expressão CEL (sqlc.yaml) |
|---|---|---|
| `bloquear-delecao-maciça` | Impede comandos DELETE ou UPDATE que não apresentem cláusulas restritivas de limite, evitando o expurgo acidental de tabelas. | `query.sql.contains("DELETE") && !query.sql.contains("WHERE")` |
| `query.sql.contains("SEQ_SCAN")` | Monitoriza o plano de execução (EXPLAIN) gerado pelo motor. Se detetar a ausência de uso de índices estatísticos (Seq Scan) associada a um custo algorítmico global inaceitável, falha a compilação garantindo estabilidade no CPU. | `postgresql.explain.plan.node_type == "Seq Scan" && postgresql.explain.plan.total_cost > 300.0` |

### 3.3. Prevenção Contra o Desvio de Esquema (Schema Drift)

Numa organização escalável com fluxos paralelos, o esquema do banco de dados (DDL) avança vertiginosamente utilizando migradores como o `golang-migrate` ou Atlas. Se um engenheiro modificar a tabela inserindo colunas sem reexecutar a ferramenta de inferência (`sqlc generate`), a divergência entre a definição em código Go e o banco de dados resulta em falhas graves de mapeamento durante o tempo de execução.

A salvaguarda arquitetural pressupõe um processo de três camadas na via de entrega contínua (CI):

1. **Refatoração Regimental:** Executar ativamente o `sqlc generate` a cada atualização de ramo de repositório.
2. **Validação Delta:** Usar `git diff --exit-code` sobre a pasta gerada para invalidar automaticamente Pull Requests cujo código gerado não acompanhe o estado modificado das queries ou esquemas (o pipeline cancela o processo ao ler o diff).
3. **Auditoria na Nuvem:** Lançar a verificação nativa da suíte, `sqlc verify`, que executa análise profunda de integridade do código da aplicação face ao planeado, assegurando que, numa base remota estrutural (esquema alvo), não existem breaking changes ocultas nas alterações submetidas à revisão.

---

## 4. Otimização Baseada na Latência e no Motor do PostgreSQL

O tempo alocado ao processamento lógico do ecossistema Go é apenas uma fração diminuta perante a fricção subjacente da entrada/saída (I/O) e latência de rede subjacente à comunicação com o PostgreSQL. Maximizar o fluxo depende da compreensão avançada do transporte de dados e do funcionamento intrínseco do Analisador Estatístico de Custos (Query Planner) do Postgres.

### 4.1. Ingestão Maciça com `COPY FROM` vs Agrupamento (Batching)

Quando o requisito dita a inserção ou atualização sistémica de milhares de linhas concorrentes (ex.: importação de transações bancárias, ficheiros CSV, rastreamento de sensores), a utilização isolada de transações sequenciais destrói a resiliência por acumulação de atrasos de processamento na camada de comunicação (TCP Round-Trips).

O driver `pgx/v5` oferece dois expedientes de remediação que são encapsulados de forma exemplar pelo SQLC:

- **Anotação `:copyfrom`:** Mapeia o comando de importação em bloco `COPY FROM` do PostgreSQL. Em vez de formatar e submeter milhares de invocações `INSERT` separadas, o código Go assimila um slice nativo (`[]Estrutura`), transformando-o eficientemente num fluxo (stream) binário unificado. O Postgres deteta o protocolo `COPY` nativo, suprime vastas etapas de análise sintática exaustiva por cada linha inserida e esvazia a memória na tabela destino num único movimento. Avaliações da indústria apontam rotineiramente a um aumento exponencial da taxa de ingestão de dados, superando até 5 vezes a velocidade das inserções tradicionais encadeadas em batches.

- **Anotação `:batch` / `:batchmany`:** Compacta múltiplos comandos SQL parametrizados sob um canal unificado, transmitendo as solicitações de uma só vez para o Postgres, e colhendo respostas sob a mesma conexão atómica.

### 4.2. A Tragédia dos Planos Genéricos nos Prepared Statements

O `pgx/v5` usa Prepared Statements em que a consulta original (a corda estática do SQL) é guardada no cache do Postgres. Quando o Go invoca a instrução, envia apenas os parâmetros atualizados (`$1, $2`), cortando drasticamente o ciclo processual.

Contudo, este benefício expõe uma idiossincrasia do otimizador de consultas (Query Planner) do PostgreSQL conhecida como a Heurística das Cinco Execuções (`plan_cache_mode = auto`).

**Fases cruciais de tomada de decisão do PostgreSQL sobre as consultas preparadas originadas pelo pacote `pgx`:**

| Fase de Execução | Estratégia do Otimizador de Consultas | Rationale de Custos Algorímicos |
|---|---|---|
| 1 a 5 | **Planos Personalizados (Custom Plan):** O Postgres cria e aplica um plano individual calculado a partir do parâmetro isolado recebido na invocação. Com acesso à variável específica, verifica os Histogramas (`pg_statistic`). Se o valor cobrir uma área exígua, recruta um Index Scan; se muito abrangente, um Seq Scan de baixo custo. Guarda-se o registo do "Custo Personalizado Médio". | O motor personaliza o plano com base nos valores reais dos parâmetros. |
| 6 (Avaliação Ponderada) | **Avaliação Comparativa:** O motor desenha, às cegas e por presunção generalista, um Plano Genérico sem olhar a valores dos parâmetros, e estima o seu custo de resolução computacional total. | O motor compara o Custo Genérico Estimado com o Custo Personalizado Médio mantido. |
| > 6 | **Ancoramento (Locked State):** Se assumir a posição de Plano Genérico, recusa-se a voltar a rever a estrutura. Toda a execução usa o mesmo percurso de indexação. | O motor cristaliza a rota. |

**A crise agudiza-se em padrões "Catch-all" de filtros opcionais descritos na secção 2.1 (ex.: `WHERE (@email IS NULL OR email = @email)`).** Nos primeiros 5 testes, passando o email devidamente preenchido, o Postgres aciona o índice das contas (Index Scan). Contudo, aquando do momento da avaliação 6, criando o Plano Genérico cego, a incapacidade estatística de aferir uma variável com dois braços independentes que admitem nulidade obriga o motor, para garantir a consistência de resolução abrangente, a adotar a conduta protetiva: ignorar os índices parciais ou absolutos e comutar a uma varredura generalista morosa (Sequential Scan). No rescaldo, após a 6.ª chamada da API pelo lado do microsserviço Go, as respostas passarão de milissegundos para dezenas de segundos sem alteração no código.

**Resolução Sistémica de Engenharia:**

Para resolver este fenómeno de de-otimização nos clusters Go:

- **Forçar Customização:** Se a variação de dados (Data Skewness) da aplicação for vasta, alterar as definições da camada de conectividade de dados de ambiente global, mudando `plan_cache_mode` de `auto` para `force_custom_plan`. Requer algum esforço contínuo no parsing da base de dados, mas garante índices seletivos robustos imutáveis.

- **Partição Comportamental no Código Go:** No domínio de engenharia relacional, as condicionantes ramificadas com `IS NULL OR` são um anti-pattern letal em tabelas com milhões de registos. Deve-se separar as condições optativas por meio de múltiplas assinaturas distintas do SQLC sem as regras opcionais (ex.: `GetByEmail`, `GetByNome`, `GetByTodos`) e executar a tomada de decisão da árvore correta ao nível superior através dos processos nativos `if/switch` da linguagem Go. A base de dados relacional funciona primariamente por travessia cartesiana unificada e a sua força baseia-se num espectro previsível e cristalino.

---

## 5. Arquitetura Subjacente de Redes: Pool de Conexões (`pgxpool`)

As fases de cold-start ou picos de carga vertiginosos da aplicação, o estabelecimento desenfreado de conexões TLS via protocolo TCP isolado esmaga a resposta latencial do Postgres (a alocação média de uma ligação excede os 20ms de rede pura). Adicionalmente, diferentemente de motores multithreaded (como o MySQL), o PostgreSQL aplica uma arquitetura multiprocesso: engendra um processo pesado no sistema operativo baseado no kernel de Linux (fork) com considerável exigência sobre a RAM, por cada conexão cliente ativa. Um desajuste neste paradigma precipitará um esgotamento brutal do anfitrião.

O agrupamento (connection pooling) proporcionado pelo pacote especializado `pgxpool` atua como salvaguarda crítica do balanceamento dos processos concorrentes do Go e da tolerância de hardware do cluster.

### 5.1. Dimensionamento Matemático do `MaxConns` e Base Mínima

**Fórmula Estrita de Limite Operativo Numérico (MaxConns):**

O teto da concorrência aplicacional máxima e estabilização paralela deverá ditar-se pela congregação das capacidades base reais do hardware que alberga o PostgreSQL:

```
MaxConns = (NumCores × 2) + EffectiveSpindleCount
```

Sendo que `EffectiveSpindleCount` remete às margens estatísticas do I/O disponível para subsistemas lógicos do núcleo do SSD. Tradicionalmente, manter um valor estrito e fechado ao limite (entre 20 a 50 processos operacionais no MaxConns por réplica do serviço Go) garantirá uma vazão otimizada em ambientes multi-tenant. Na ultrapassagem da contenção, a aplicação Go repousa e pausa as solicitações no canal, aguardando que outras goroutines resolvam as mutações sem gerar novos processos anfitriões de colapso, protegendo a subsistência central.

**Manutenção Latencial Mínima (MinConns / MinIdleConns):**

Por norma, é contraproducente terminar e destruir a sessão de persistência entre períodos de menor tráfego (ou nas quebras dos vales). Para erradicar a penosa e mortífera latência na reinicialização de conexões quando regressam surtos aleatórios (bursts), devemos definir limites de base passiva com o MinConns entre 10% a 25% da quota total. Ao dispor deste reservatório permanente pré-aquecido e sincronizado (warm connections), a aplicação exibe uma prestação imperturbável.

### 5.2. Prevenção do Thundering Herd e Higiene Periódica (Healthcheck)

Com limites predefinidos de duração absoluta da conexão (`MaxConnLifetime` ou equivalente nas variantes da abstração `database/sql`), as sessões obsoletas que atingem o tempo máximo estipulado (ex.: 3 horas) exigem terminação profilática de encerramento gradual de memória de longo curso. Contudo, caso ocorra a estipulação uniforme num reinício ou lançamento (deploy), atingida a duração exata dessa terceira hora de laboração contínua de processamento, as conexões da aplicação implodem todas simuladamente e na retaguarda, disparando pedidos furiosos cumulativos (reconexões ao Postgres de forma assíncrona). Trata-se da vulnerabilidade crítica classificada como o efeito do Rebanho Em Fúria (Thundering Herd).

O sistema do `pgxpool` fornece uma variação caótica vital denominada `MaxConnLifetimeJitter`. Este flutuador integra aleatoriedade matemática adicional aos períodos de abstenção. Assim, a desconexão dilui-se no espectro de tempo não linear das horas posteriores e impede as fixações do encerramento forçado de pacotes ou as mudanças nos clusters mestre das infraestruturas (Failover/Failback routing), suavizando os gráficos operacionais aplicacionais.

Aliado a processos periódicos com o HealthCheckPeriod, assegura-se a identificação contínua e abate passivo profilático de instâncias que ficaram deslaçadas nas quedas de reencaminhamento persistentes sem bloquear a aplicação no limiar transacional.

---

## 6. Arquitetura Aplicacional (DDD) e Desacoplamento Transacional

O código resultante das iterações de geração por SQLC apresenta-se inequivocamente engavetado e acoplado sob o paradigma descritivo puramente persistente. Modelar as regras e imutáveis da lógica aplicacional (as condicionais estritas do domínio) diretamente nas predefinições abstratas exangues destas respostas traduz as falácias mortíferas nos contextos regidos do Clean Architecture.

### 6.1. A Camada Mapeadora (Data Mappers) do Domain-Driven Design

No escopo e ideologia protetora enformada pelas doutrinas de conceção orientadas pelo Domínio (DDD), as Unidades Entitárias primárias (Entities e Aggregates) requerem e subscrevem validações, métodos comportamentais invioláveis e construtores herméticos (como validação de balanço de finanças não inferior ao zero absoluto e e-mails regulados sem acesso externo explícito dos atributos estruturais nas pastas nativas com nomes confidenciais da linguagem).

O subproduto gerado e cuspido na estrutura nativa por parte da configuração da máquina estática do SQLC limita-se unicamente a materializar o repositório mecânico plano. Pertence sem reticências ou hesitação ao invólucro (Port) do Adaptador Exterior nos estratos Hexagonais. O ecossistema de aplicação requer um Padrão Mapeador Intermediário (Data Mapper).

O Repository em Go ingere nativamente as operações, recolhe e extrai o modelo do repositório (ex.: o espelho do DDL `db.OrderRow`), instanciando, de seguida, a passagem pela fábrica controladora (`Constructor Factory domain.NewOrder`) com preservação inviolável para o núcleo interno do software isolado das amarras infraestruturais, blindando e resguardando a base limpa aplicacional perante eventuais reestruturações futuras da camada de persistência.

### 6.2. Atomição e Encapsulamento de Transações Multicíclicas

Comandos intrinsecamente correlacionados e interdependentes cuja execução exige sequenciações interligadas (como o lançamento contínuo de montantes duplos entre contas credoras e devedoras nas plataformas bancárias), implicam forçosamente o aglutinamento rigoroso através da coesão no encapsulamento estrito e absoluto Relacional de Transações (Transactions). A fragmentação sem invólucro atómico redundará na geração impura e caótica de persistências anómalas nas paralizações elétricas eventuais de um nó distribuído, deixando os estados em ruínas parciais irrecuperáveis.

O sistema transicional estatuído internamente pelas interações com o compilador baseia-se num abstrator basilar flexível nominado na interface abstrata relacional `DBTX`. A passagem unificada na arquitetura do controlador do serviço garante que o repositório é dotado do construto semântico referenciado explicitamente como método instancial do `WithTx`:

```go
func (store *SQLStore) TransferenciaAtomica(ctx context.Context, args TransferParams) error {
    // 1. Instanciar transação com bloqueio e níveis isolativos rígidos.
    tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{
        IsoLevel: pgx.Serializable,
    })
    if err != nil {
        return err
    }

    // Assegura imperativamente reversão de falha por contexto escapado (panic)
    defer tx.Rollback(ctx)

    // 2. Acoplar e substituir motor basilar abstrato de interface DBTX.
    qtx := store.Queries.WithTx(tx)

    // 3. Aplicação Múltipla Paralela (Encapsulada na Sessão Protegida).
    if err := qtx.DebitAccount(ctx, args.OrigemID, args.Valor); err != nil {
        return err // Aciona reversão deferida automatizada (Rollback)
    }
    if err := qtx.CreditAccount(ctx, args.DestinoID, args.Valor); err != nil {
        return err
    }

    // Subscrever persistência real (Commit final).
    return tx.Commit(ctx)
}
```

O PostgreSQL defende uma implementação flexível nos sistemas transacionais (TxOptions), cujas definições determinam quão cega será a intervenção concorrente ao ecossistema paralelo. Definir os níveis (Isolation Levels) ajustados do driver (como a salvaguarda absoluta do nível `Serializable`) confina mutações em processos críticos onde condições de corrida fantasma provocam destruições graves ao registo dos capitais, suprimindo o Dirty Read e falhas na base de observação de forma incontornável em transações de repetição crítica interlaçada.

### 6.3. Cobertura Analítica com Testes Unitários e Interfaces Mock

Assentar lógicas efémeras pesadas aplicacionais ao rigor cego das requisições cíclicas permanentes numa instalação emulação de contentor nas plataformas da integração distribuída (Docker no GitHub Actions/GitLab) retarda a libertação funcional paralela exponencialmente no Continuous Delivery. Se todas as verificações do fluxo exaustivo lógico dependerem unicamente dessa simulação integral das transações lentas, os feedbacks recuarão na perceção operacional.

Habilitar a anotação supracitada da arquitetura (`emit_interface: true`) no DDL emit_interface contorna estruturalmente estas debilidades. A plataforma SQLC condensa todas as implementações numa única grelha tipificada referenciada estritamente como a interface `Querier`. Quando combinada e abstraída no invólucro do construtor de gestão, permite o auxílio e integração de ferramentas como a formatação modular mockgen do ramo `gomock` e dependências estáticas. O processo gera implementações (stubs e mocks) isoladas para a arquitetura onde a camada lógica estipula, via chamadas como `EXPECT().Method().Return(...)`, os controlos predeterminados da simulação virtual, descartando as ligações base reais morosas do sistema.

O Mock do Store assegura cobertura unitária total nas ramificações lógicas dos serviços puros Go em ambiente fechado de processador e sem infraestrutura subjacente de dependência persistente (base de dados isolada para verificação pontual efémera local).

---

## 7. Conclusões Práticas e Sumário

A adoção do SQLC com o PostgreSQL em arquiteturas Go robustas oferece garantias ímpares de estabilidade transacional, segurança defensiva integrada através de preceções invioláveis dos protocolos estáticos do SQL (Bind), e rendimento exponencial comparativo sustentado pelas vias binárias do protocolo base do `pgx/v5`.

A consolidação de práticas experientes assenta fundamentalmente nos seguintes pilares cruciais:

1. **Configuração Explícita:** Definir controlos fortes com `pgx/v5` habilitado na topologia, substituindo nulos indesejados (`emit_pointers_for_null_types: true`) e anulando dependências defuntas não aplicadas pela via `omit_unused_structs`. Promover o uso dos arrays nativos PostgreSQL (`ANY(...)`) rejeitando veementemente a via instável gerada da expansão com o macro primitivo `sqlc.slice`.

2. **Gestão Heurística do Analisador:** Repelir lógicas condicionais flexíveis exaustivas no ramo estático SQL (`IS NULL OR`) e, perante mutações drásticas temporais (queda latencial na sexta execução cega da rotina das análises estatísticas `plan_cache_mode`), quebrar ativamente a estrutura unificada optando por invocações independentes delineáveis via o modelo decisional base da camada do servidor de rede Go.

3. **Dimensionalidade Restrita Operacional:** Restringir o afluxo descontrolado nas camadas latenciais balizando cirurgicamente os topes do PostgreSQL no estrato `pgxpool`, dimensionando a fórmula algorítmica associada à estrutura multicore transacional e minimizando as derrocadas caóticas e encerramentos em catadupa com dispersão ruidosa (`MaxConnLifetimeJitter`).

4. **Implementar estes controlos converte aplicações monolíticas em sistemas altamente previsíveis e indestrutíveis sob volumes extensivos.** A infraestrutura deixa de dominar os processos negociais do software e recua unicamente para a base da estabilidade atómica aplicacional orientada pela engenharia estrutural dos modelos limpos (Clean Models).

---

## Revisões do ADR

- **ADR-01-Ledger-SQLC:** Proposta criada
- **Próximos passos:** Validar com a equipa de engenharia o uso do SQLC na base de código existente, ajustar `sqlc.yaml` conforme as configurações práticas do projeto, e garantir que `sqlc vet` e `sqlc generate` no pipeline CI/CD funcionam corretamente.
- **Responsável:** [Nome do responsável]
- **Data de criação:** 2026-08-12