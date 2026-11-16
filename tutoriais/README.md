# AtlasCloud IaaS — Protótipo Executável dos Tutoriais (Grupo 6)

**Disciplina:** Modelagem de Padrões de Projeto / Engenharia de Software III (IES-300) — Fatec Araraquara (2º Semestre de 2026)  
**Docente:** Prof. Me. Ronaldo Dametto  
**Integrantes (Grupo 6):** Lucas Harb, Luigi Hisatsuga, Luiz Gustavo, Pedro Alves e Rhyan Dos Anjos Andrade  
**Repositório Git:** https://github.com/Rhyan121121/atlascloud-iaas  

---

## 1. Tema e Núcleo Implementado

O **AtlasCloud IaaS** (originalmente denominado *HeapState* no Checkpoint 1 e unificado para *AtlasCloud IaaS* a partir da Oficina 02) é uma plataforma de computação em nuvem no modelo *Infrastructure as a Service* (IaaS) multi-região, concebida para o provisionamento sob demanda de máquinas virtuais (VMs), redes isoladas (VPCs), regras de firewall (*Security Groups*) e governança financeira em tempo real (*FinOps*). O sistema resolve o problema da alocação segura, elástica e auditável de recursos computacionais compartilhados entre múltiplos locatários (*multi-tenant*), evitando sobreposição de capacidade física, vazamento de tráfego entre sub-redes e estouros imprevistos de orçamento.

Como o domínio completo de um provedor IaaS abrange mais de 200 serviços e comunicação física com *hypervisors* KVM, o grupo delimitou como **núcleo executável** o plano de controle de orquestração composto por três recursos centrais — **Instâncias Virtuais (`/v1/instances`)**, **Security Groups (`/v1/security-groups`)** e **Orçamentos por Conta (`/v1/budgets`)** — integrados pelas quatro tecnologias da disciplina em **Go (Golang)**: (1) **API REST** nível 2 de Richardson com validação rigorosa e erros padronizados em *Problem Details* (RFC 9457); (2) **Mensageria assíncrona no RabbitMQ 4** com *exchange* `topic`, dois consumidores independentes (Notificações e Auditoria), nova tentativa limitada (*retry*), *Dead Letter Queue* (DLQ) e controle de idempotência; (3) **Servidor GraphQL (BFF)** com resolução aninhada em três níveis, *DataLoader* por requisição e *Mutations* tipadas; e (4) **API Gateway resiliente** na borda com autenticação por chave de API, planos de uso (*Rate Limiting*), *Timeout* estrito de 2 segundos, *Circuit Breaker* (disjuntor) e rota agregada com degradação graciosa.

---

## 2. Pré-requisitos

* **Docker** e **Docker Compose** em execução (sobe todo o ambiente — RabbitMQ 4 e os 6 microsserviços em Go — com um único comando).
* **Go (Golang) 1.22 ou superior** *(opcional, apenas caso deseje compilar e rodar os binários nativamente fora do Docker via `./dev.sh`)*.
* **`curl`** e **`python3`** (utilizados pelos scripts de demonstração na pasta `testes/`).

> **Segredos e Variáveis de Ambiente:** Nenhuma chave real consta no repositório. Consulte o arquivo [`.env.example`](.env.example) para visualizar as variáveis de configuração padrão.

---

## 3. Como Executar (Passo a Passo e Tabela de Processos e Portas)

### Opção A — Execução Unificada via Docker Compose (Recomendada — Ponto Extra)

Na pasta `tutoriais/` (ou na raiz do pacote), execute um único comando:

```bash
docker compose up -d --build
```

Aguarde cerca de 10 segundos para que o *healthcheck* do RabbitMQ libere os microsserviços. Para verificar se todos estão saudáveis:

```bash
docker compose ps
curl -i http://localhost:8080/healthz
```

### Opção B — Execução Local dos Processos em Go (Sem Docker para as APIs)

Caso deseje rodar apenas o RabbitMQ no Docker e os 6 serviços diretamente no sistema operacional:

```bash
# 1. Subir apenas o broker RabbitMQ 4
docker compose up -d rabbitmq

# 2. Compilar e iniciar os 6 processos Go em background (ouvindo em 127.0.0.1)
./dev.sh start

# 3. Conferir a tabela de processos e portas
./dev.sh status
```

### Tabela de Processos, Portas e Isolamento de Rede

| Processo / Contêiner | Pasta do Código | Porta / Protocolo | Endereço de Escuta / Visibilidade | Responsabilidade Principal |
| :--- | :--- | :--- | :--- | :--- |
| **`atlas-gateway`** | `gateway/` + `frontend/` | **`8080` (HTTP)** | **`0.0.0.0:8080` (Única porta pública)** | Autentica, limita uso, aplica disjuntor, reescreve `Location`, agrega dashboard e serve o Console Web em `/`. |
| **`atlas-compute`** | `rest/compute/` | `8081` (HTTP) | `127.0.0.1:8081` (Rede interna) | API REST de Instâncias Virtuais (CRUD completo, filtros, paginação, regras de negócio e publicação AMQP). |
| **`atlas-security`** | `rest/security/` | `8082` (HTTP) | `127.0.0.1:8082` (Rede interna) | API REST de Security Groups e Firewall (validação de portas e bloqueio de exclusão se em uso `409`). |
| **`atlas-billing`** | `rest/billing/` | `8083` (HTTP) | `127.0.0.1:8083` (Rede interna) | API REST de Tarifação e Orçamento por conta; publica `billing.charge.recorded` e `billing.budget.exceeded`. |
| **`atlas-workers`** | `mensageria/` | `AMQP 0-9-1` (`5672`) | Rede interna (Cliente RabbitMQ) | Executa **2 consumidores** em filas distintas (`atlas.notifications.queue` e `atlas.audit.queue`) com `Prefetch(1)`, `Ack` manual, *Retry* (3x), DLQ (`atlas.dlq`) e Idempotência. |
| **`atlas-graphql`** | `graphql/` | `8084` (HTTP) | `127.0.0.1:8084` (Rede interna) | Servidor GraphQL BFF com *Schema* tipado, *DataLoader* por requisição, consultas aninhadas e *Mutations*. |
| **`atlas-rabbitmq`** | `compose.yaml` | `5672` / `15672` | `localhost:15672` (Painel Web) | Broker RabbitMQ 4.x (`guest` / `guest`) com exchange `atlas.events` (`topic`) e `atlas.dlx` (`direct`). |

---

## 4. Chaves de API e Usuários de Teste (Todos Fictícios)

O API Gateway exige o cabeçalho `X-API-Key` em todas as rotas de API (`/api/v1/*`, `/v1/*`, `/api/graphql`). As chaves nunca são repassadas aos serviços internos (que recebem apenas `X-Cliente-Id` e `X-Request-Id`):

| Cliente / Ator do Tema | Cabeçalho `X-API-Key` (Fictício) | Limite por Minuto | Autorização de Rotas |
| :--- | :--- | :--- | :--- |
| **Console Web do Cliente** (`console-web`) | `console-web` | **40 req / min** | Acesso completo às rotas de Compute, Security, Billing, Dashboard e GraphQL. |
| **Automação IaC Terraform** (`cli-terraform`) | `cli-terraform` | **120 req / min** | Plano de alta vazão para provisionamento declarativo e scripts de carga. |
| **Parceiro de Auditoria FinOps** (`parceiro-auditoria`) | `parceiro-auditoria` | **30 req / min** | **Restrito (Ponto Extra `403`):** Acessa apenas `/api/v1/budgets` e `/api/v1/security-groups`. Recebe `403 Forbidden` em `/api/v1/instances`. |

* **Painel Web do RabbitMQ:** `http://localhost:15672` — Usuário: `guest` | Senha: `guest`
* **Console Web AtlasCloud IaaS:** `http://localhost:8080`
* **Conta Locatária Padrão de Teste:** `acc-fatec-01`

---

## 5. Roteiro da Demonstração Ao Vivo (As 8 Cenas Obrigatórias)

Todos os comandos abaixo possuem scripts prontos na pasta [`testes/`](testes/). Para executar todas as 8 cenas em sequência de forma automatizada:

```bash
./testes/executar_todas_as_cenas.sh
```

Ou execute cena por cena durante a apresentação:

### Cena 1 — Ambiente no ar (`./testes/cena01_ambiente_no_ar.sh`)
Mostra os processos rodando, a tabela de portas e comprova que o Gateway responde na porta pública `8080`:
```bash
./testes/cena01_ambiente_no_ar.sh
```

### Cena 2 — API REST pelo Gateway (`./testes/cena02_api_rest_regras_negocio.sh`)
Cria um Security Group e uma Instância Virtual (`201 Created` com cabeçalho `Location: /api/v1/instances/{id}` reescrito pelo Gateway), lê o item (`200`), lista com filtro, ordenação e paginação (`?status=RUNNING&flavor=c5.large&pagina=1&limite=5&ordenar=name&ordem=asc`), substitui com `PUT` (`200`) e provoca as **3 regras de negócio do domínio** em formato *Problem Details* (`application/problem+json`):
1. Alterar `flavor` de instância em estado `RUNNING` → **`422 Unprocessable Content`**
2. Criar instância ativa com nome duplicado na mesma VPC → **`409 Conflict`**
3. Excluir Security Group vinculado a instância ativa → **`409 Conflict`** (além da trava de cota máxima de 8 vCPUs → **`422 Unprocessable Content`**)
```bash
./testes/cena02_api_rest_regras_negocio.sh
```

### Cena 3 — Evento no RabbitMQ e 2 Consumidores (`./testes/cena03_evento_rabbitmq_consumidores.sh`)
Ao criar uma instância ou registrar cobrança, a API responde imediatamente e publica na exchange `atlas.events` (`topic`). Mostra nos logs que:
* Eventos críticos (`compute.instance.created`, `billing.budget.exceeded`) são processados tanto pelo **Consumidor 1 (Notificações — fila `atlas.notifications.queue`)** quanto pelo **Consumidor 2 (Auditoria — fila `atlas.audit.queue`)**;
* Eventos rotineiros de tarifação (`billing.charge.recorded`) são roteados exclusivamente para o **Consumidor 2 (Auditoria, padrão `#`)**:
```bash
./testes/cena03_evento_rabbitmq_consumidores.sh
```

### Cena 4 — Falha, Nova Tentativa Limitada, DLQ e Idempotência (`./testes/cena04_falha_retry_dlq_idempotencia.sh`)
Demonstra os três mecanismos de resiliência assíncrona:
1. **Idempotência:** Publica o mesmo evento duas vezes com o mesmo `event_id`; a 2ª entrega é descartada com `[IDEMPOTENCY_SKIP]` sem reenviar e-mail.
2. **Nova tentativa limitada (`x-tentativas`):** Publica mensagem com falha simulada; o consumidor tenta 3 vezes (`[RETRY] tentativa 1/3`, `2/3`) e, na 3ª falha, envia para a **DLQ (`atlas.dlq`)** com `[DLQ_REJECT]`.
3. **Poison Message:** Publica JSON malformado, que é rejeitado imediatamente (`Nack(requeue=false)`) para a `atlas.dlq`.
```bash
./testes/cena04_falha_retry_dlq_idempotencia.sh
```

### Cena 5 — Servidor GraphQL (`./testes/cena05_graphql_aninhamento_mutation.sh`)
Executa no endpoint `/api/graphql`:
1. **Mutation com `input` (`CreateInstanceInput`):** Cria uma VM via GraphQL repassando `X-Request-Id` para a API REST.
2. **Query da Tela Principal (`accountDashboard`):** Busca em uma única consulta os dados de Conta, Orçamento, Instâncias, Security Groups e Regras de Firewall (3 níveis de aninhamento resolvidos com **DataLoader por requisição**), exibindo no campo `restCallsAvoided` quantas chamadas REST foram evitadas.
3. **Tratamento de Erro em Mutation:** Envia um `flavor` inválido e recebe `errors` com `extensions.code = "BAD_USER_INPUT"` e `httpStatus = 422`.
```bash
./testes/cena05_graphql_aninhamento_mutation.sh
```

### Cena 6 — Proteções do API Gateway (`./testes/cena06_protecoes_gateway.sh`)
Comprova todas as defesas de borda em formato *Problem Details*:
* Sem chave de API → **`401 Unauthorized`**
* Chave `parceiro-auditoria` tentando acessar `/api/v1/instances` → **`403 Forbidden`** (Autorização por rota)
* Excedendo o limite do plano `console-web` → **`429 Too Many Requests`** com cabeçalho `Retry-After`
* Serviço lento (`?atraso=2600` > timeout de 2000ms) → **`504 Gateway Timeout`**
* Serviço desligado → 3 respostas **`502 Bad Gateway`**, abrindo o **Circuit Breaker (Disjuntor)** que passa a responder imediatamente com **`503 Service Unavailable`** + `Retry-After`. Ao religar o serviço, demonstra o fechamento automático do disjuntor (**`200 OK`**).
```bash
./testes/cena06_protecoes_gateway.sh
```

### Cena 7 — Rota Agregada com Degradação Graciosa (`./testes/cena07_rota_agregada_degradacao.sh`)
Consulta `GET /api/v1/dashboard/acc-fatec-01`:
1. Com todos os serviços no ar → `status: "HEALTHY"`, `parcial: false`.
2. Com o serviço de Billing desligado → o Gateway não falha; devolve `200 OK` com os dados de Compute e Security preenchidos, `billing: null`, `status: "DEGRADED"`, `parcial: true` e `degraded_services: ["billing"]`.
```bash
./testes/cena07_rota_agregada_degradacao.sh
```

### Cena 8 — Fechamento e Rastreamento Ponta a Ponta (`./testes/cena08_fechamento_rastreabilidade.sh`)
Envia uma requisição e exibe o **mesmo `X-Request-Id`** correlacionado simultaneamente no cabeçalho de resposta do cliente, no log do **API Gateway**, no log da **API REST (`atlas-compute`)** e nos logs dos **2 Consumidores RabbitMQ (`atlas-workers`)**:
```bash
./testes/cena08_fechamento_rastreabilidade.sh
```

---

## 6. Decisões de Arquitetura e Limitações do Protótipo (Resumo)

* **Por que cada tecnologia está no tema:**
  * **API REST (`rest/`):** Provê contratos padronizados por recurso (`/v1/instances`, `/v1/security-groups`, `/v1/budgets`) com semântica HTTP estrita (RFC 9110) e erros estruturados (RFC 9457), ideal para automação via Terraform (`run-terraform.sh`).
  * **Mensageria RabbitMQ (`mensageria/`):** Desacopla temporalmente o provisionamento de VMs do disparo de alertas de *FinOps* e da gravação de trilhas de auditoria, absorvendo picos de carga com `Prefetch(1)`, *Retry* limitado e DLQ.
  * **GraphQL (`graphql/`):** Elimina o *under-fetching* e o problema $N+1$ no carregamento do painel consolidado do cliente através de resolução aninhada com *DataLoader* por requisição.
  * **API Gateway (`gateway/`):** Centraliza preocupações transversais (autenticação `X-API-Key`, autorização por rota, *rate limiting* por plano, *timeout* de 2s, *circuit breaker* e correlação `X-Request-Id`) e isola os microsserviços em rede interna (`127.0.0.1`).
* **Principais limitações do protótipo e evolução para produção (detalhadas no `relatorio-tutoriais.pdf`):**
  1. **Armazenamento em memória (`sync.RWMutex`):** Em produção, seria substituído por PostgreSQL multi-AZ com chaves UUIDv7 e controle de concorrência otimista/pessimista (`SELECT FOR UPDATE`).
  2. **Contadores de Rate Limit e Idempotência em processo único:** Em produção com múltiplas réplicas do Gateway e dos Workers, seriam externalizados para um cluster **Redis** compartilhado e tabela de deduplicação transacional.
  3. **Publicação de eventos fora de transação (Escrita Dupla):** Em produção, seria aplicado o padrão **Transactional Outbox** (Richardson, 2018) para garantir atomicidade entre a gravação no banco e a publicação no broker.
  4. **Cabeçalho `X-Cliente-Id` em texto puro na rede interna:** Em produção, a comunicação leste-oeste entre Gateway e microsserviços utilizaria **mTLS** (*Service Mesh*) ou tokens JWT internos assinados.

---

## 7. Divisão de Responsabilidades (Grupo 6)

Todos os integrantes participaram da concepção arquitetural, revisão da documentação das oficinas e integração final:

| Integrante | Responsabilidade Principal no Projeto e nos Tutoriais | Marco / Tag no Git |
| :--- | :--- | :--- |
| **Rhyan Dos Anjos Andrade** | Parte 4: API Gateway (`gateway/`), autenticação, planos de uso, *Circuit Breaker*, rota agregada, Docker Compose e Orquestração Terraform. | `etapa-4-gateway` / `entrega-final` |
| **Pedro Alves** | Parte 1: API REST de Compute (`rest/compute/`), CRUD completo, paginação, validações RFC 9457 e regras de cota/flavor/VPC. | `etapa-1-rest` |
| **Lucas Harb** | Parte 1 e Documentação: API REST de Security Groups (`rest/security/`), contrato `openapi.yaml` e modelagem C4/GRASP das Oficinas. | `etapa-1-rest` |
| **Luiz Gustavo** | Parte 2: Mensageria (`mensageria/` e `rest/billing/`), integração RabbitMQ 4, 2 consumidores, *Retry*, DLQ e Idempotência. | `etapa-2-mensageria` |
| **Luigi Hisatsuga** | Parte 3 e Integração: Servidor GraphQL BFF (`graphql/`), *DataLoader*, *Mutations*, Console Web (`frontend/`) e scripts de teste (`testes/`). | `etapa-3-graphql` / `entrega-final` |

---

## 8. Declaração de Uso de Ferramentas de Inteligência Artificial

O modelo **Gemini (Google)** foi utilizado como ferramenta de apoio para geração de rascunhos de código boilerplate em Go, estruturação dos scripts de teste automatizados em Bash, diagramação visual dos slides e revisão gramatical/ABNT do relatório técnico. O grupo revisou, testou e compreendeu integralmente todo o código entregue, tendo rejeitado abordagens genéricas com banco acoplado ao GraphQL ou componentes batizados com sufixos de classes (`Manager`/`Controller`), mantendo estrita aderência às restrições EARS, ao modelo C4 das oficinas e aos quatro tutoriais da disciplina.

---

## 9. Links do Repositório Git

* **Repositório GitHub (com tags `etapa-1-rest`, `etapa-2-mensageria`, `etapa-3-graphql`, `etapa-4-gateway`, `entrega-final`):**  
  https://github.com/Rhyan121121/atlascloud-iaas

---

## 10. Referências Bibliográficas

* BROWN, S. **The C4 model for visualising software architecture**. 2026. Disponível em: https://c4model.com.
* FIELDING, R. T.; NOTTINGHAM, M.; RESCHKE, J. **RFC 9110: HTTP Semantics**. IETF, 2022.
* HOHPE, G.; WOOLF, B. **Enterprise Integration Patterns: Designing, Building, and Deploying Messaging Solutions**. Boston: Addison-Wesley, 2003.
* ISO/IEC. **ISO/IEC 25010:2023 — Systems and software engineering — SQuaRE — Product quality model**. 2. ed. Genebra: ISO, 2023.
* KLEPPMANN, M. **Designing Data-Intensive Applications**. Sebastopol: O'Reilly Media, 2017.
* LARMAN, C. **Utilizando UML e padrões: introdução à análise e ao projeto orientados a objetos**. 3. ed. Porto Alegre: Bookman, 2007.
* MAVIN, A. et al. **Easy Approach to Requirements Syntax (EARS)**. In: *17th IEEE International Requirements Engineering Conference*, 2009.
* NOTTINGHAM, M.; WILDE, E.; DALAL, S. **RFC 9457: Problem Details for HTTP APIs**. IETF, jul. 2023.
* NYGARD, M. T. **Release It! Design and Deploy Production-Ready Software**. 2. ed. Raleigh: Pragmatic Bookshelf, 2018.
* RICHARDS, M.; FORD, N. **Fundamentals of Software Architecture: A Modern Engineering Approach**. 2. ed. Sebastopol: O'Reilly Media, 2025.
* RICHARDSON, C. **Microservices Patterns: With Examples in Java**. Shelter Island: Manning, 2018.
