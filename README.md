# Pacote de Entrega Final — Grupo 6: AtlasCloud IaaS

**Disciplina:** Modelagem de Padrões de Projeto / Engenharia de Software III (Fatec Araraquara — 2º Sem. 2026)  
**Docente:** Prof. Me. Ronaldo Dametto  
**Integrantes:** Lucas Harb, Luigi Hisatsuga, Luiz Gustavo, Pedro Alves e Rhyan Dos Anjos Andrade  
**Repositório Git:** https://github.com/Rhyan121121/atlascloud-iaas  

---

## Estrutura Oficial do Pacote (`grupo-06-atlascloud-iaas.zip`)

Este diretório está organizado rigorosamente conforme a **Seção 5** do documento de especificação da Entrega Final do Bimestre:

```text
grupo-06-atlascloud-iaas/
├── 00-indice.pdf             ← Índice completo de todos os documentos, datas, autores e revisões
├── documentacao/             ← Toda a documentação arquitetural produzida nas oficinas do semestre
│   ├── oficina-01/           ← Checkpoint 1 / Oficina 01: ASRs, ISO/IEC 25010:2023 e Cenários (unificado para AtlasCloud IaaS)
│   └── oficina-02/           ← Entrega 2 (Oficinas 1 e 2): C4 Nível 3, 7 Componentes GRASP, 8 Restrições EARS e ADR-004
├── tutoriais/                ← Código executável dos 4 tutoriais aplicados ao tema AtlasCloud IaaS
│   ├── README.md             ← Instruções completas de execução, tabela de portas e roteiro das 8 cenas
│   ├── compose.yaml          ← Sobe TODO o ambiente (RabbitMQ 4 + 6 microsserviços Go) com 1 comando
│   ├── openapi.yaml          ← Especificação OpenAPI 3.1.0 com respostas RFC 9457 Problem Details
│   ├── rest/                 ← Parte 1: APIs REST do domínio (compute/, security/, billing/)
│   ├── mensageria/           ← Parte 2: Consumidores RabbitMQ (Notificações e Auditoria, Retry, DLQ e Idempotência)
│   ├── graphql/              ← Parte 3: Servidor GraphQL BFF (Schema, Queries aninhadas, DataLoader e Mutations)
│   ├── gateway/              ← Parte 4: API Gateway (Autenticação, Planos, 429, 504, 502, Disjuntor 503 e Rota Agregada)
│   ├── frontend/             ← Console Web interativo servido pelo Gateway em http://localhost:8080
│   ├── terraform/            ← Orquestração declarativa IaC (Infraestrutura como Código)
│   └── testes/               ← Scripts automatizados das 8 cenas obrigatórias da demonstração
├── relatorio-tutoriais.pdf   ← Relatório Técnico de Integração e Decisões de Arquitetura (Parte 5)
└── apresentacao.pdf          ← Slides da Apresentação Oral (Bloco 1: Documentos + Bloco 2: Demo Ao Vivo)
```

## Execução Rápida (1 Comando)

Você pode subir todo o ambiente tanto a partir desta raiz quanto de dentro da pasta `tutoriais/`:

```bash
docker compose up -d --build
```

E para executar a bateria completa das **8 cenas oficiais da demonstração**:

```bash
./tutoriais/testes/executar_todas_as_cenas.sh
```

Consulte [`tutoriais/README.md`](tutoriais/README.md) para o guia completo passo a passo.
