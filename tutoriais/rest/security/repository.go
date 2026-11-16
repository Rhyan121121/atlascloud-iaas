package main

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound       = errors.New("security group not found")
	ErrInvalidPort    = errors.New("port must be between 1 and 65535, with start_port <= end_port")
	ErrInvalidProto   = errors.New("invalid protocol, allowed: TCP, UDP, ICMP")
	ErrMissingFields  = errors.New("name and vpc_id are required")
)

type Repository struct {
	mu sync.RWMutex
	sg map[string]*SecurityGroup
}

func NewRepository() *Repository {
	return &Repository{
		sg: make(map[string]*SecurityGroup),
	}
}

func (r *Repository) Create(req CreateSecurityGroupRequest) (*SecurityGroup, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.VPCID) == "" {
		return nil, ErrMissingFields
	}

	var parsedRules []FirewallRule
	for _, rule := range req.Rules {
		proto := strings.ToUpper(strings.TrimSpace(rule.Protocol))
		if proto != "TCP" && proto != "UDP" && proto != "ICMP" {
			return nil, ErrInvalidProto
		}

		if proto != "ICMP" {
			if rule.StartPort < 1 || rule.StartPort > 65535 || rule.EndPort < 1 || rule.EndPort > 65535 || rule.StartPort > rule.EndPort {
				return nil, ErrInvalidPort
			}
		}

		cidr := strings.TrimSpace(rule.CIDROrigin)
		if cidr == "" {
			cidr = "0.0.0.0/0"
		}

		rID, _ := uuid.NewV7()
		rIDStr := rID.String()
		if rIDStr == "" {
			rIDStr = uuid.NewString()
		}

		parsedRules = append(parsedRules, FirewallRule{
			ID:         rIDStr,
			Protocol:   proto,
			StartPort:  rule.StartPort,
			EndPort:    rule.EndPort,
			CIDROrigin: cidr,
		})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	sgID, _ := uuid.NewV7()
	sgIDStr := sgID.String()
	if sgIDStr == "" {
		sgIDStr = uuid.NewString()
	}

	accountID := req.AccountID
	if accountID == "" {
		accountID = "acc-fatec-01"
	}

	now := time.Now().UTC()
	group := &SecurityGroup{
		ID:        sgIDStr,
		Name:      req.Name,
		VPCID:     req.VPCID,
		AccountID: accountID,
		Rules:     parsedRules,
		CreatedAt: now,
		UpdatedAt: now,
	}

	r.sg[sgIDStr] = group
	return group, nil
}

func (r *Repository) GetByID(id string) (*SecurityGroup, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	group, ok := r.sg[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *group
	return &cp, nil
}

func (r *Repository) List(vpcID, accountID string) []SecurityGroup {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []SecurityGroup
	for _, group := range r.sg {
		if vpcID != "" && group.VPCID != vpcID {
			continue
		}
		if accountID != "" && group.AccountID != accountID {
			continue
		}
		result = append(result, *group)
	}
	if result == nil {
		result = []SecurityGroup{}
	}
	return result
}

func (r *Repository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.sg[id]; !ok {
		return ErrNotFound
	}
	delete(r.sg, id)
	return nil
}
