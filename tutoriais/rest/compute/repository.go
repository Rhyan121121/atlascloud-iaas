package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound            = errors.New("instance not found")
	ErrFlavorRunning       = errors.New("cannot change flavor of an instance in RUNNING status")
	ErrQuotaExceeded       = errors.New("account vCPU quota exceeded")
	ErrDuplicateActiveName = errors.New("an active instance with this name already exists in this VPC")
	ErrInvalidFlavor       = errors.New("invalid flavor, allowed: t3.micro, c5.large")
)

const (
	AccountVCPUQuota = 8  // Cota contratual da conta: 8 vCPUs (Restrição EARS [R7])
	MaxPageLimit     = 50 // Teto máximo de itens por página (OWASP API4)
)

type Repository struct {
	mu        sync.RWMutex
	instances map[string]*Instance
	ipCounter int
}

func NewRepository() *Repository {
	return &Repository{
		instances: make(map[string]*Instance),
		ipCounter: 10,
	}
}

func (r *Repository) Create(req CreateInstanceRequest) (*Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	flavorSpec, ok := FlavorSpecs[req.Flavor]
	if !ok {
		return nil, ErrInvalidFlavor
	}

	accountID := req.AccountID
	if accountID == "" {
		accountID = "acc-fatec-01"
	}

	// 1. Validar cota de vCPUs da conta (somente instâncias ativas: PROVISIONING e RUNNING)
	currentVCPUs := 0
	for _, inst := range r.instances {
		if inst.AccountID == accountID && (inst.Status == StatusRunning || inst.Status == StatusProvisioning) {
			currentVCPUs += inst.VCPUs
		}
	}
	if currentVCPUs+flavorSpec.VCPUs > AccountVCPUQuota {
		return nil, ErrQuotaExceeded
	}

	// 2. Validar nome duplicado na mesma VPC (para instâncias não terminadas)
	for _, inst := range r.instances {
		if inst.VPCID == req.VPCID && inst.Status != StatusTerminated && strings.EqualFold(inst.Name, req.Name) {
			return nil, ErrDuplicateActiveName
		}
	}

	// 3. Gerar ID imutável (UUIDv7 conforme ADR-004 e Restrição EARS [R1])
	id, err := uuid.NewV7()
	var idStr string
	if err == nil {
		idStr = id.String()
	} else {
		idStr = uuid.NewString()
	}

	r.ipCounter++
	privIP := fmt.Sprintf("10.0.1.%d", r.ipCounter)
	pubIP := fmt.Sprintf("54.210.12.%d", r.ipCounter)
	now := time.Now().UTC() // Carimbo estrito em UTC ISO 8601 conforme Restrição EARS [R2]

	inst := &Instance{
		ID:              idStr,
		Name:            req.Name,
		Flavor:          req.Flavor,
		Status:          StatusRunning,
		IPPrivado:       privIP,
		IPPublico:       pubIP,
		VPCID:           req.VPCID,
		SecurityGroupID: req.SecurityGroupID,
		AccountID:       accountID,
		VCPUs:           flavorSpec.VCPUs,
		MemoryGB:        flavorSpec.MemoryGB,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	r.instances[idStr] = inst
	return inst, nil
}

func (r *Repository) GetByID(id string) (*Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	inst, ok := r.instances[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *inst
	return &cp, nil
}

func (r *Repository) List(status, flavor, vpcID, sgID, accountID, search string, page, limit int, sortBy, order string) PaginatedInstancesResponse {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []Instance
	for _, inst := range r.instances {
		if status != "" && !strings.EqualFold(string(inst.Status), status) {
			continue
		}
		if flavor != "" && !strings.EqualFold(inst.Flavor, flavor) {
			continue
		}
		if vpcID != "" && inst.VPCID != vpcID {
			continue
		}
		if sgID != "" && inst.SecurityGroupID != sgID {
			continue
		}
		if accountID != "" && inst.AccountID != accountID {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(inst.Name), strings.ToLower(search)) {
			continue
		}
		filtered = append(filtered, *inst)
	}

	// Ordenação
	sort.Slice(filtered, func(i, j int) bool {
		if strings.EqualFold(sortBy, "name") || strings.EqualFold(sortBy, "nome") {
			if strings.EqualFold(order, "asc") {
				return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
			}
			return strings.ToLower(filtered[i].Name) > strings.ToLower(filtered[j].Name)
		}
		if strings.EqualFold(sortBy, "vcpus") {
			if strings.EqualFold(order, "asc") {
				return filtered[i].VCPUs < filtered[j].VCPUs
			}
			return filtered[i].VCPUs > filtered[j].VCPUs
		}
		// Default por created_at
		if strings.EqualFold(order, "asc") {
			return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	total := len(filtered)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > MaxPageLimit {
		limit = MaxPageLimit
	}

	pages := (total + limit - 1) / limit
	if pages == 0 {
		pages = 1
	}

	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	items := filtered[start:end]
	if items == nil {
		items = []Instance{}
	}

	return PaginatedInstancesResponse{
		Items: items,
		Dados: items,
		Total: total,
		Page:  page,
		Limit: limit,
		Pages: pages,
		Paginacao: PaginationMeta{
			Pagina:       page,
			Limite:       limit,
			Total:        total,
			TotalPaginas: pages,
		},
	}
}

// Replace substitui integralmente os dados da instância (PUT /v1/instances/{id})
func (r *Repository) Replace(id string, req CreateInstanceRequest) (*Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, ok := r.instances[id]
	if !ok {
		return nil, ErrNotFound
	}

	newSpec, ok := FlavorSpecs[req.Flavor]
	if !ok {
		return nil, ErrInvalidFlavor
	}

	// Regra a: Não permitir alterar flavor se a instância estiver em estado RUNNING (422)
	if req.Flavor != inst.Flavor && inst.Status == StatusRunning {
		return nil, ErrFlavorRunning
	}

	accountID := req.AccountID
	if accountID == "" {
		accountID = inst.AccountID
	}

	// Regra b: Validar cota de vCPUs da conta
	currentVCPUs := 0
	for _, other := range r.instances {
		if other.ID != id && other.AccountID == accountID && (other.Status == StatusRunning || other.Status == StatusProvisioning) {
			currentVCPUs += other.VCPUs
		}
	}
	if (inst.Status == StatusRunning || inst.Status == StatusProvisioning) && currentVCPUs+newSpec.VCPUs > AccountVCPUQuota {
		return nil, ErrQuotaExceeded
	}

	// Regra c: Validar nome duplicado na mesma VPC
	for _, other := range r.instances {
		if other.ID != id && other.VPCID == req.VPCID && other.Status != StatusTerminated && strings.EqualFold(other.Name, req.Name) {
			return nil, ErrDuplicateActiveName
		}
	}

	inst.Name = req.Name
	inst.Flavor = req.Flavor
	inst.VPCID = req.VPCID
	inst.SecurityGroupID = req.SecurityGroupID
	inst.AccountID = accountID
	inst.VCPUs = newSpec.VCPUs
	inst.MemoryGB = newSpec.MemoryGB
	inst.UpdatedAt = time.Now().UTC()

	cp := *inst
	return &cp, nil
}

func (r *Repository) Update(id string, req UpdateInstanceRequest) (*Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, ok := r.instances[id]
	if !ok {
		return nil, ErrNotFound
	}

	// Regra a: Não permitir alterar flavor de instância em estado RUNNING (422)
	if req.Flavor != nil && *req.Flavor != inst.Flavor {
		if inst.Status == StatusRunning {
			return nil, ErrFlavorRunning
		}
		newSpec, ok := FlavorSpecs[*req.Flavor]
		if !ok {
			return nil, ErrInvalidFlavor
		}
		inst.Flavor = *req.Flavor
		inst.VCPUs = newSpec.VCPUs
		inst.MemoryGB = newSpec.MemoryGB
	}

	if req.Name != nil && *req.Name != inst.Name {
		for _, other := range r.instances {
			if other.ID != id && other.VPCID == inst.VPCID && other.Status != StatusTerminated && strings.EqualFold(other.Name, *req.Name) {
				return nil, ErrDuplicateActiveName
			}
		}
		inst.Name = *req.Name
	}

	if req.Status != nil {
		oldStatus := inst.Status
		newStatus := *req.Status
		inst.Status = newStatus

		if newStatus == StatusStopped || newStatus == StatusTerminated {
			inst.IPPublico = ""
		} else if newStatus == StatusRunning && oldStatus != StatusRunning {
			if inst.IPPublico == "" {
				r.ipCounter++
				inst.IPPublico = fmt.Sprintf("54.210.12.%d", r.ipCounter)
			}
		}
	}

	inst.UpdatedAt = time.Now().UTC()
	cp := *inst
	return &cp, nil
}

func (r *Repository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, ok := r.instances[id]
	if !ok {
		return ErrNotFound
	}

	inst.Status = StatusTerminated
	inst.IPPublico = ""
	inst.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *Repository) PurgeTerminated(accountID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for id, inst := range r.instances {
		if (accountID == "" || inst.AccountID == accountID) && inst.Status == StatusTerminated {
			delete(r.instances, id)
			count++
		}
	}
	return count
}

func (r *Repository) PurgeByID(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.instances[id]; !ok {
		return ErrNotFound
	}
	delete(r.instances, id)
	return nil
}
