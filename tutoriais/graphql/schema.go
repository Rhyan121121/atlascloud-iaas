package main

import (
	"atlascloud/pkg/httputil"
	"strings"
	"sync"

	"github.com/graphql-go/graphql"
)

type contextKey string

const DataLoaderContextKey contextKey = "sg_dataloader"

// GraphQLCodedError implementa graphql.ExtendedError para incluir extensions.code (padrão Apollo Server 5)
type GraphQLCodedError struct {
	Message    string
	Code       string
	HTTPStatus int
	RequestID  string
}

func (e *GraphQLCodedError) Error() string {
	return e.Message
}

func (e *GraphQLCodedError) Extensions() map[string]interface{} {
	ext := map[string]interface{}{
		"code": e.Code,
	}
	if e.HTTPStatus > 0 {
		ext["httpStatus"] = e.HTTPStatus
	}
	if e.RequestID != "" {
		ext["requestId"] = e.RequestID
	}
	return ext
}

func mapStatusToGraphQLCode(httpStatus int) string {
	switch httpStatus {
	case 400, 422:
		return "BAD_USER_INPUT"
	case 404:
		return "NOT_FOUND"
	case 409:
		return "CONFLICT"
	default:
		return "INTERNAL_SERVER_ERROR"
	}
}

var instanceStatusEnum = graphql.NewEnum(graphql.EnumConfig{
	Name:        "InstanceStatus",
	Description: "Estados possíveis do ciclo de vida de uma instância virtual no AtlasCloud IaaS",
	Values: graphql.EnumValueConfigMap{
		"PROVISIONING": &graphql.EnumValueConfig{Value: "PROVISIONING"},
		"RUNNING":      &graphql.EnumValueConfig{Value: "RUNNING"},
		"STOPPED":      &graphql.EnumValueConfig{Value: "STOPPED"},
		"TERMINATED":   &graphql.EnumValueConfig{Value: "TERMINATED"},
	},
})

var createInstanceInputType = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "CreateInstanceInput",
	Description: "Dados de entrada para provisionar uma nova instância virtual via Mutation",
	Fields: graphql.InputObjectConfigFieldMap{
		"name": &graphql.InputObjectFieldConfig{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Nome único da instância na VPC",
		},
		"flavor": &graphql.InputObjectFieldConfig{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Flavor de hardware (t3.micro ou c5.large)",
		},
		"vpcId": &graphql.InputObjectFieldConfig{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Identificador da VPC",
		},
		"securityGroupId": &graphql.InputObjectFieldConfig{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Identificador do Security Group vinculado",
		},
		"accountId": &graphql.InputObjectFieldConfig{
			Type:        graphql.String,
			Description: "Identificador da conta locatária (default: acc-fatec-01)",
		},
	},
})

var firewallRuleType = graphql.NewObject(graphql.ObjectConfig{
	Name: "FirewallRule",
	Fields: graphql.Fields{
		"id":         &graphql.Field{Type: graphql.String},
		"protocol":   &graphql.Field{Type: graphql.String},
		"startPort":  &graphql.Field{Type: graphql.Int},
		"endPort":    &graphql.Field{Type: graphql.Int},
		"cidrOrigin": &graphql.Field{Type: graphql.String},
	},
})

var securityGroupType = graphql.NewObject(graphql.ObjectConfig{
	Name: "SecurityGroup",
	Fields: graphql.Fields{
		"id":        &graphql.Field{Type: graphql.String},
		"name":      &graphql.Field{Type: graphql.String},
		"vpcId":     &graphql.Field{Type: graphql.String},
		"accountId": &graphql.Field{Type: graphql.String},
		"rules": &graphql.Field{
			Type: graphql.NewList(firewallRuleType),
		},
	},
})

func mapSecurityGroupDTO(g SecurityGroupDTO) map[string]interface{} {
	var rules []map[string]interface{}
	for _, r := range g.Rules {
		rules = append(rules, map[string]interface{}{
			"id":         r.ID,
			"protocol":   r.Protocol,
			"startPort":  r.StartPort,
			"endPort":    r.EndPort,
			"cidrOrigin": r.CIDROrigin,
		})
	}
	return map[string]interface{}{
		"id":        g.ID,
		"name":      g.Name,
		"vpcId":     g.VPCID,
		"accountId": g.AccountID,
		"rules":     rules,
	}
}

func mapInstanceDTO(inst InstanceDTO) map[string]interface{} {
	return map[string]interface{}{
		"id":              inst.ID,
		"name":            inst.Name,
		"flavor":          inst.Flavor,
		"status":          inst.Status,
		"ipPrivado":       inst.IPPrivado,
		"ipPublico":       inst.IPPublico,
		"vpcId":           inst.VPCID,
		"securityGroupId": inst.SecurityGroupID,
		"accountId":       inst.AccountID,
		"vcpus":           inst.VCPUs,
		"memoryGb":        inst.MemoryGB,
		"createdAt":       inst.CreatedAt,
	}
}

func BuildSchema(client *RestClient) (graphql.Schema, error) {
	instanceType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Instance",
		Fields: graphql.Fields{
			"id":              &graphql.Field{Type: graphql.String},
			"name":            &graphql.Field{Type: graphql.String},
			"flavor":          &graphql.Field{Type: graphql.String},
			"status":          &graphql.Field{Type: graphql.String},
			"ipPrivado":       &graphql.Field{Type: graphql.String},
			"ipPublico":       &graphql.Field{Type: graphql.String},
			"vpcId":           &graphql.Field{Type: graphql.String},
			"securityGroupId": &graphql.Field{Type: graphql.String},
			"accountId":       &graphql.Field{Type: graphql.String},
			"vcpus":           &graphql.Field{Type: graphql.Int},
			"memoryGb":        &graphql.Field{Type: graphql.Int},
			"createdAt":       &graphql.Field{Type: graphql.String},
			// Campo aninhado (2º e 3º níveis do grafo: Instance -> SecurityGroup -> FirewallRule) resolvido via DataLoader!
			"securityGroup": &graphql.Field{
				Type:        securityGroupType,
				Description: "Security Group associado à instância, carregado via DataLoader por requisição (evita N+1)",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					m, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, nil
					}
					sgID, _ := m["securityGroupId"].(string)
					if sgID == "" {
						return nil, nil
					}
					if loader, ok := p.Context.Value(DataLoaderContextKey).(*SecurityGroupDataLoader); ok && loader != nil {
						g, err := loader.Load(p.Context, sgID)
						if err != nil || g == nil {
							return nil, nil
						}
						return mapSecurityGroupDTO(*g), nil
					}
					return nil, nil
				},
			},
		},
	})

	budgetType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Budget",
		Fields: graphql.Fields{
			"accountId":      &graphql.Field{Type: graphql.String},
			"currentCost":    &graphql.Field{Type: graphql.Float},
			"budgetLimit":    &graphql.Field{Type: graphql.Float},
			"currency":       &graphql.Field{Type: graphql.String},
			"exceeded":       &graphql.Field{Type: graphql.Boolean},
			"forecastedCost": &graphql.Field{Type: graphql.Float},
		},
	})

	accountDashboardType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AccountDashboard",
		Fields: graphql.Fields{
			"accountId":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"totalInstances":   &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"runningInstances": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"totalVCPUs":       &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"restCallsAvoided": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"instances": &graphql.Field{
				Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(instanceType))),
			},
			"securityGroups": &graphql.Field{
				Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(securityGroupType))),
			},
			"budget": &graphql.Field{
				Type: budgetType,
			},
		},
	})

	rootQuery := graphql.NewObject(graphql.ObjectConfig{
		Name: "RootQuery",
		Fields: graphql.Fields{
			// 1. Consulta Agregadora da Tela Principal (substitui 3+ chamadas REST em 1 única requisição GraphQL)
			"accountDashboard": &graphql.Field{
				Type: accountDashboardType,
				Args: graphql.FieldConfigArgument{
					"accountId": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.String),
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					accountID := p.Args["accountId"].(string)
					ctx := p.Context
					reqID := ""
					if v, ok := ctx.Value(httputil.RequestIDContextKey).(string); ok {
						reqID = v
					}

					var wg sync.WaitGroup
					wg.Add(3)

					var instances []InstanceDTO
					var sgs []SecurityGroupDTO
					var budget *BudgetDTO

					go func() {
						defer wg.Done()
						insts, err := client.FetchInstances(ctx, accountID, reqID)
						if err == nil {
							instances = insts
						}
					}()

					go func() {
						defer wg.Done()
						groups, err := client.FetchSecurityGroups(ctx, accountID, reqID)
						if err == nil {
							sgs = groups
						}
					}()

					go func() {
						defer wg.Done()
						b, err := client.FetchBudget(ctx, accountID, reqID)
						if err == nil {
							budget = b
						}
					}()

					wg.Wait()

					// Abastece o DataLoader da requisição com os SecurityGroups já buscados (0 chamadas extras no resolver!)
					if loader, ok := ctx.Value(DataLoaderContextKey).(*SecurityGroupDataLoader); ok && loader != nil {
						loader.PrimeAll(sgs)
					}

					totalInst := len(instances)
					runningInst := 0
					totalVCPUs := 0
					for _, inst := range instances {
						if inst.Status == "RUNNING" {
							runningInst++
							totalVCPUs += inst.VCPUs
						}
					}

					sgResult := make([]map[string]interface{}, 0, len(sgs))
					for _, g := range sgs {
						sgResult = append(sgResult, mapSecurityGroupDTO(g))
					}

					instResult := make([]map[string]interface{}, 0, len(instances))
					for _, inst := range instances {
						instResult = append(instResult, mapInstanceDTO(inst))
					}

					var bResult map[string]interface{}
					if budget != nil {
						bResult = map[string]interface{}{
							"accountId":      budget.AccountID,
							"currentCost":    budget.CurrentCost,
							"budgetLimit":    budget.BudgetLimit,
							"currency":       budget.Currency,
							"exceeded":       budget.Exceeded,
							"forecastedCost": budget.ForecastedCost,
						}
					}

					// Uma chamada GraphQL substitui: 1 (instances) + 1 (budget) + max(1, len(instances)) (security groups) chamadas REST!
					restCallsAvoided := 2 + len(instances)
					if restCallsAvoided < 3 {
						restCallsAvoided = 3
					}

					return map[string]interface{}{
						"accountId":        accountID,
						"totalInstances":   totalInst,
						"runningInstances": runningInst,
						"totalVCPUs":       totalVCPUs,
						"restCallsAvoided": restCallsAvoided,
						"instances":        instResult,
						"securityGroups":   sgResult,
						"budget":           bResult,
					}, nil
				},
			},

			// 2. Consulta de Instâncias com argumentos de filtro (accountId, status, flavor)
			"instances": &graphql.Field{
				Type: graphql.NewList(instanceType),
				Args: graphql.FieldConfigArgument{
					"accountId": &graphql.ArgumentConfig{Type: graphql.String},
					"status":    &graphql.ArgumentConfig{Type: graphql.String},
					"flavor":    &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					accountID, _ := p.Args["accountId"].(string)
					status, _ := p.Args["status"].(string)
					flavor, _ := p.Args["flavor"].(string)
					reqID := ""
					if v, ok := p.Context.Value(httputil.RequestIDContextKey).(string); ok {
						reqID = v
					}
					list, err := client.FetchInstancesFiltered(p.Context, accountID, status, flavor, reqID)
					if err != nil {
						return []map[string]interface{}{}, nil
					}
					res := make([]map[string]interface{}, 0, len(list))
					for _, i := range list {
						res = append(res, mapInstanceDTO(i))
					}
					return res, nil
				},
			},

			// 3. Consulta individual de Instância por ID com resolução aninhada de SecurityGroup
			"instance": &graphql.Field{
				Type: instanceType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					id, _ := p.Args["id"].(string)
					reqID := ""
					if v, ok := p.Context.Value(httputil.RequestIDContextKey).(string); ok {
						reqID = v
					}
					inst, err := client.FetchInstanceByID(p.Context, id, reqID)
					if err != nil || inst == nil {
						return nil, nil
					}
					return mapInstanceDTO(*inst), nil
				},
			},

			// 4. Consulta de Security Groups com filtro opcional por vpcId
			"securityGroups": &graphql.Field{
				Type: graphql.NewList(securityGroupType),
				Args: graphql.FieldConfigArgument{
					"accountId": &graphql.ArgumentConfig{Type: graphql.String},
					"vpcId":     &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					accountID, _ := p.Args["accountId"].(string)
					vpcID, _ := p.Args["vpcId"].(string)
					reqID := ""
					if v, ok := p.Context.Value(httputil.RequestIDContextKey).(string); ok {
						reqID = v
					}
					groups, err := client.FetchSecurityGroups(p.Context, accountID, reqID)
					if err != nil {
						return []map[string]interface{}{}, nil
					}
					res := make([]map[string]interface{}, 0, len(groups))
					for _, g := range groups {
						if vpcID != "" && !strings.EqualFold(g.VPCID, vpcID) {
							continue
						}
						res = append(res, mapSecurityGroupDTO(g))
					}
					return res, nil
				},
			},
		},
	})

	// RootMutation com tipo Input e tratamento de erros com GraphQLError + extensions.code
	rootMutation := graphql.NewObject(graphql.ObjectConfig{
		Name: "RootMutation",
		Fields: graphql.Fields{
			"createInstance": &graphql.Field{
				Type:        instanceType,
				Description: "Provisiona uma nova instância virtual via API REST repassando X-Request-Id",
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(createInstanceInputType),
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					inputMap, _ := p.Args["input"].(map[string]interface{})
					reqID := ""
					if v, ok := p.Context.Value(httputil.RequestIDContextKey).(string); ok {
						reqID = v
					}

					accountID, _ := inputMap["accountId"].(string)
					if accountID == "" {
						accountID = "acc-fatec-01"
					}

					payload := map[string]interface{}{
						"name":              inputMap["name"],
						"flavor":            inputMap["flavor"],
						"vpc_id":            inputMap["vpcId"],
						"security_group_id": inputMap["securityGroupId"],
						"account_id":        accountID,
					}

					inst, status, detail, err := client.CreateInstance(p.Context, payload, reqID)
					if err != nil {
						return nil, &GraphQLCodedError{
							Message:    detail,
							Code:       mapStatusToGraphQLCode(status),
							HTTPStatus: status,
							RequestID:  reqID,
						}
					}
					return mapInstanceDTO(*inst), nil
				},
			},

			"updateInstanceStatus": &graphql.Field{
				Type:        instanceType,
				Description: "Altera o estado (RUNNING, STOPPED, TERMINATED) de uma instância virtual",
				Args: graphql.FieldConfigArgument{
					"id":     &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"status": &graphql.ArgumentConfig{Type: graphql.NewNonNull(instanceStatusEnum)},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					id, _ := p.Args["id"].(string)
					status, _ := p.Args["status"].(string)
					reqID := ""
					if v, ok := p.Context.Value(httputil.RequestIDContextKey).(string); ok {
						reqID = v
					}

					inst, httpStatus, detail, err := client.UpdateInstanceStatus(p.Context, id, status, reqID)
					if err != nil {
						return nil, &GraphQLCodedError{
							Message:    detail,
							Code:       mapStatusToGraphQLCode(httpStatus),
							HTTPStatus: httpStatus,
							RequestID:  reqID,
						}
					}
					return mapInstanceDTO(*inst), nil
				},
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{
		Query:    rootQuery,
		Mutation: rootMutation,
	})
}
