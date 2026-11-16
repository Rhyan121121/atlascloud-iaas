package main

import (
	"time"
)

type InstanceStatus string

const (
	StatusProvisioning InstanceStatus = "PROVISIONING"
	StatusRunning      InstanceStatus = "RUNNING"
	StatusStopped      InstanceStatus = "STOPPED"
	StatusTerminated   InstanceStatus = "TERMINATED"
)

type FlavorInfo struct {
	VCPUs    int
	MemoryGB int
	PriceSec float64
}

var FlavorSpecs = map[string]FlavorInfo{
	"t3.micro": {VCPUs: 1, MemoryGB: 1, PriceSec: 0.02 / 3600.0},
	"c5.large": {VCPUs: 2, MemoryGB: 4, PriceSec: 0.10 / 3600.0},
}

// Instance representa uma Máquina Virtual no AtlasCloud IaaS
type Instance struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Flavor          string         `json:"flavor"`
	Status          InstanceStatus `json:"status"`
	IPPrivado       string         `json:"ip_privado"`
	IPPublico       string         `json:"ip_publico"`
	VPCID           string         `json:"vpc_id"`
	SecurityGroupID string         `json:"security_group_id"`
	AccountID       string         `json:"account_id"`
	VCPUs           int            `json:"vcpus"`
	MemoryGB        int            `json:"memory_gb"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type CreateInstanceRequest struct {
	Name            string `json:"name"`
	Flavor          string `json:"flavor"`
	VPCID           string `json:"vpc_id"`
	SecurityGroupID string `json:"security_group_id"`
	AccountID       string `json:"account_id"`
}

type UpdateInstanceRequest struct {
	Name   *string         `json:"name,omitempty"`
	Flavor *string         `json:"flavor,omitempty"`
	Status *InstanceStatus `json:"status,omitempty"`
}

type PaginationMeta struct {
	Pagina       int `json:"pagina"`
	Limite       int `json:"limite"`
	Total        int `json:"total"`
	TotalPaginas int `json:"totalPaginas"`
}

type PaginatedInstancesResponse struct {
	Items     []Instance     `json:"items"`
	Dados     []Instance     `json:"dados"`
	Total     int            `json:"total"`
	Page      int            `json:"page"`
	Limit     int            `json:"limit"`
	Pages     int            `json:"pages"`
	Paginacao PaginationMeta `json:"paginacao"`
}
