package main

import "time"

// FirewallRule representa uma regra de tráfego de entrada/saída
type FirewallRule struct {
	ID         string `json:"id"`
	Protocol   string `json:"protocol"` // TCP, UDP, ICMP
	StartPort  int    `json:"start_port"`
	EndPort    int    `json:"end_port"`
	CIDROrigin string `json:"cidr_origin"` // Ex: 0.0.0.0/0
}

// SecurityGroup representa o grupo de firewall virtual
type SecurityGroup struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	VPCID     string         `json:"vpc_id"`
	AccountID string         `json:"account_id"`
	Rules     []FirewallRule `json:"rules"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type CreateRuleRequest struct {
	Protocol   string `json:"protocol"`
	StartPort  int    `json:"start_port"`
	EndPort    int    `json:"end_port"`
	CIDROrigin string `json:"cidr_origin"`
}

type CreateSecurityGroupRequest struct {
	Name      string              `json:"name"`
	VPCID     string              `json:"vpc_id"`
	AccountID string              `json:"account_id"`
	Rules     []CreateRuleRequest `json:"rules"`
}
