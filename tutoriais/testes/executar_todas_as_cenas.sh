#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

echo "################################################################################"
echo "#   ATLASCLOUD IAAS - EXECUÇÃO AUTOMATIZADA DAS 8 CENAS OFICIAIS DO EDITAL     #"
echo "#   Fatec Araraquara - Modelagem de Padrões de Projeto / ES III (Grupo 6)      #"
echo "################################################################################"
echo ""

bash "$DIR/cena01_ambiente_no_ar.sh"
echo ""
bash "$DIR/cena02_api_rest_regras_negocio.sh"
echo ""
bash "$DIR/cena03_evento_rabbitmq_consumidores.sh"
echo ""
bash "$DIR/cena04_falha_retry_dlq_idempotencia.sh"
echo ""
bash "$DIR/cena05_graphql_aninhamento_mutation.sh"
echo ""
bash "$DIR/cena06_protecoes_gateway.sh"
echo ""
bash "$DIR/cena07_rota_agregada_degradacao.sh"
echo ""
bash "$DIR/cena08_fechamento_rastreabilidade.sh"

echo ""
echo "################################################################################"
echo "# ✔ TODAS AS 8 CENAS DO ROTEIRO OFICIAL EXECUTADAS E VALIDADAS COM SUCESSO!    #"
echo "################################################################################"
