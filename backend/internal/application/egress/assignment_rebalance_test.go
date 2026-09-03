package egress

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	domain "github.com/chenyme/grok2api/backend/internal/domain/egress"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

type stubEgressNodes struct {
	nodes []domain.Node
}

func (s *stubEgressNodes) ListEgressNodes(context.Context, domain.Scope, repository.SortQuery) ([]domain.Node, error) {
	return s.nodes, nil
}
func (s *stubEgressNodes) GetEgressNode(context.Context, uint64) (domain.Node, error) {
	return domain.Node{}, ErrNotFound
}
func (s *stubEgressNodes) CreateEgressNode(context.Context, domain.Node) (domain.Node, error) {
	return domain.Node{}, nil
}
func (s *stubEgressNodes) UpdateEgressNode(context.Context, domain.Node) (domain.Node, error) {
	return domain.Node{}, nil
}
func (s *stubEgressNodes) DeleteEgressNode(context.Context, uint64) error {
	return nil
}
func (s *stubEgressNodes) ListEgressNodePage(context.Context, repository.EgressNodeListQuery) ([]domain.Node, int64, error) {
	return nil, 0, nil
}

type bindingUpdate struct {
	provider accountdomain.Provider
	ids      []uint64
	nodeID   *uint64
	mode     accountdomain.EgressAssignmentMode
}

type stubAccountBindings struct {
	accounts map[accountdomain.Provider][]accountdomain.Credential
	updates  []bindingUpdate
}

func (s *stubAccountBindings) ListEgressAssignments(_ context.Context, provider accountdomain.Provider) ([]accountdomain.Credential, error) {
	return s.accounts[provider], nil
}

func (s *stubAccountBindings) UpdateEgressBindings(_ context.Context, provider accountdomain.Provider, ids []uint64, nodeID *uint64, mode accountdomain.EgressAssignmentMode, _ time.Time) (int64, error) {
	s.updates = append(s.updates, bindingUpdate{provider: provider, ids: append([]uint64(nil), ids...), nodeID: nodeID, mode: mode})
	return int64(len(ids)), nil
}

func (s *stubAccountBindings) CountProviderAccountsByIDs(context.Context, accountdomain.Provider, []uint64) (int64, error) {
	return 0, nil
}
func (s *stubAccountBindings) ListEgressBindingProviders(context.Context, uint64) ([]accountdomain.Provider, error) {
	return nil, nil
}
func (s *stubAccountBindings) ListEgressSourceBindingProviders(context.Context, uint64) ([]accountdomain.Provider, error) {
	return nil, nil
}

func healthyEgressNode(id uint64, scope domain.Scope, capacity, assigned int) domain.Node {
	now := time.Now().UTC()
	return domain.Node{
		ID: id, Scope: scope, Enabled: true, EncryptedProxyURL: "http://proxy.example:8080",
		AccountCapacity: capacity, AssignedAccountCount: assigned,
		ProbeStatus: domain.ProbeStatusHealthy, LastProbedAt: &now,
	}
}

func unhealthyEgressNode(id uint64, scope domain.Scope, capacity, assigned int) domain.Node {
	node := healthyEgressNode(id, scope, capacity, assigned)
	node.ProbeStatus = domain.ProbeStatusUnhealthy
	return node
}

func activeWebAccount(id uint64, mode accountdomain.EgressAssignmentMode, nodeID uint64, assignedAt *time.Time) accountdomain.Credential {
	return accountdomain.Credential{
		ID: id, Provider: accountdomain.ProviderWeb, Enabled: true, AuthStatus: accountdomain.AuthStatusActive,
		EgressNodeID: nodeID, EgressAssignmentMode: mode, EgressAssignedAt: assignedAt,
	}
}

func oldEnough() *time.Time {
	value := time.Now().UTC().Add(-10 * time.Minute)
	return &value
}

func movedIDs(updates []bindingUpdate) []uint64 {
	var ids []uint64
	for _, update := range updates {
		ids = append(ids, update.ids...)
	}
	return ids
}

func TestRebalanceAccountsPlacesUnboundAccountsOnLeastLoadedHealthyNode(t *testing.T) {
	nodes := &stubEgressNodes{nodes: []domain.Node{
		healthyEgressNode(2, domain.ScopeWeb, 0, 5),
		healthyEgressNode(1, domain.ScopeWeb, 0, 0),
	}}
	accounts := &stubAccountBindings{accounts: map[accountdomain.Provider][]accountdomain.Credential{
		accountdomain.ProviderWeb: {
			activeWebAccount(11, "", 0, nil),
			activeWebAccount(12, "", 0, nil),
			activeWebAccount(13, "", 0, nil),
		},
	}}
	service := &Service{repository: nodes, accounts: accounts}

	result, err := service.RebalanceAccounts(context.Background(), true, false, time.Minute)
	if err != nil {
		t.Fatalf("RebalanceAccounts() error = %v", err)
	}
	if result.Assigned != 3 || result.Rebalanced != 0 || result.Unplaced != 0 {
		t.Fatalf("RebalanceAccounts() = %+v, want all three accounts assigned", result)
	}
	if len(accounts.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one binding batch", accounts.updates)
	}
	update := accounts.updates[0]
	if update.nodeID == nil || *update.nodeID != 1 {
		t.Fatalf("update node = %v, want least-loaded node 1", update.nodeID)
	}
	if update.mode != accountdomain.EgressAssignmentAuto {
		t.Fatalf("update mode = %q, want auto", update.mode)
	}
	if len(update.ids) != 3 {
		t.Fatalf("update ids = %v, want all three accounts", update.ids)
	}
}

func TestRebalanceAccountsDoesNotAssignUnboundWhenAutoAssignDisabled(t *testing.T) {
	nodes := &stubEgressNodes{nodes: []domain.Node{healthyEgressNode(1, domain.ScopeWeb, 0, 0)}}
	accounts := &stubAccountBindings{accounts: map[accountdomain.Provider][]accountdomain.Credential{
		accountdomain.ProviderWeb: {activeWebAccount(11, "", 0, nil)},
	}}
	service := &Service{repository: nodes, accounts: accounts}

	result, err := service.RebalanceAccounts(context.Background(), false, true, time.Minute)
	if err != nil {
		t.Fatalf("RebalanceAccounts() error = %v", err)
	}
	if result != (RebalanceResult{}) {
		t.Fatalf("RebalanceAccounts() = %+v, want no movement", result)
	}
	if len(accounts.updates) != 0 {
		t.Fatalf("unexpected updates = %+v", accounts.updates)
	}
}

func TestRebalanceAccountsEvacuatesAutoBoundAccountsFromUnhealthyNode(t *testing.T) {
	nodes := &stubEgressNodes{nodes: []domain.Node{
		unhealthyEgressNode(6, domain.ScopeWeb, 0, 2),
		healthyEgressNode(5, domain.ScopeWeb, 0, 0),
	}}
	accounts := &stubAccountBindings{accounts: map[accountdomain.Provider][]accountdomain.Credential{
		accountdomain.ProviderWeb: {
			activeWebAccount(20, accountdomain.EgressAssignmentAuto, 6, oldEnough()),
			activeWebAccount(21, accountdomain.EgressAssignmentAuto, 6, oldEnough()),
			activeWebAccount(22, accountdomain.EgressAssignmentManual, 6, nil),
		},
	}}
	service := &Service{repository: nodes, accounts: accounts}

	result, err := service.RebalanceAccounts(context.Background(), true, false, time.Minute)
	if err != nil {
		t.Fatalf("RebalanceAccounts() error = %v", err)
	}
	if result.Rebalanced != 2 {
		t.Fatalf("RebalanceAccounts() = %+v, want two evacuations", result)
	}
	moved := movedIDs(accounts.updates)
	if len(moved) != 2 || containsID(moved, 22) || !containsID(moved, 20) || !containsID(moved, 21) {
		t.Fatalf("moved ids = %v, want accounts 20 and 21 only", moved)
	}
	for _, update := range accounts.updates {
		if update.nodeID == nil || *update.nodeID != 5 {
			t.Fatalf("update node = %v, want healthy node 5", update.nodeID)
		}
	}
}

func TestRebalanceAccountsNeverMovesManualBindings(t *testing.T) {
	nodes := &stubEgressNodes{nodes: []domain.Node{
		healthyEgressNode(1, domain.ScopeWeb, 1, 1),
		healthyEgressNode(2, domain.ScopeWeb, 0, 0),
	}}
	accounts := &stubAccountBindings{accounts: map[accountdomain.Provider][]accountdomain.Credential{
		accountdomain.ProviderWeb: {activeWebAccount(30, accountdomain.EgressAssignmentManual, 1, nil)},
	}}
	service := &Service{repository: nodes, accounts: accounts}

	// Node 1 is over capacity, but its only binding is manual: nothing may move.
	result, err := service.RebalanceAccounts(context.Background(), true, true, time.Minute)
	if err != nil {
		t.Fatalf("RebalanceAccounts() error = %v", err)
	}
	if result != (RebalanceResult{}) {
		t.Fatalf("RebalanceAccounts() = %+v, want no movement", result)
	}
	if len(accounts.updates) != 0 {
		t.Fatalf("manual binding was touched: %+v", accounts.updates)
	}
}

func TestRebalanceAccountsRepairsOverCapacityEvenWithoutAutoBalance(t *testing.T) {
	nodes := &stubEgressNodes{nodes: []domain.Node{
		healthyEgressNode(1, domain.ScopeWeb, 1, 3),
		healthyEgressNode(2, domain.ScopeWeb, 0, 0),
	}}
	accounts := &stubAccountBindings{accounts: map[accountdomain.Provider][]accountdomain.Credential{
		accountdomain.ProviderWeb: {
			activeWebAccount(31, accountdomain.EgressAssignmentAuto, 1, oldEnough()),
			activeWebAccount(32, accountdomain.EgressAssignmentAuto, 1, oldEnough()),
			activeWebAccount(33, accountdomain.EgressAssignmentAuto, 1, oldEnough()),
		},
	}}
	service := &Service{repository: nodes, accounts: accounts}

	result, err := service.RebalanceAccounts(context.Background(), true, false, time.Minute)
	if err != nil {
		t.Fatalf("RebalanceAccounts() error = %v", err)
	}
	if result.Rebalanced != 2 {
		t.Fatalf("RebalanceAccounts() = %+v, want two capacity repairs", result)
	}
	moved := movedIDs(accounts.updates)
	if len(moved) != 2 || !containsID(moved, 31) || !containsID(moved, 32) || containsID(moved, 33) {
		t.Fatalf("moved ids = %v, want exactly accounts 31 and 32", moved)
	}
}

func TestRebalanceAccountsCapacityRepairRespectsMigrationCooldown(t *testing.T) {
	fresh := time.Now().UTC()
	nodes := &stubEgressNodes{nodes: []domain.Node{
		healthyEgressNode(1, domain.ScopeWeb, 1, 3),
		healthyEgressNode(2, domain.ScopeWeb, 0, 0),
	}}
	accounts := &stubAccountBindings{accounts: map[accountdomain.Provider][]accountdomain.Credential{
		accountdomain.ProviderWeb: {
			activeWebAccount(31, accountdomain.EgressAssignmentAuto, 1, &fresh),
			activeWebAccount(32, accountdomain.EgressAssignmentAuto, 1, &fresh),
			activeWebAccount(33, accountdomain.EgressAssignmentAuto, 1, &fresh),
		},
	}}
	service := &Service{repository: nodes, accounts: accounts}

	result, err := service.RebalanceAccounts(context.Background(), true, false, time.Minute)
	if err != nil {
		t.Fatalf("RebalanceAccounts() error = %v", err)
	}
	if result.Rebalanced != 0 || len(accounts.updates) != 0 {
		t.Fatalf("RebalanceAccounts() = %+v, updates = %+v; fresh assignments must not be migrated", result, accounts.updates)
	}
}

func TestRebalanceAccountsMigrationShareCapsEvacuation(t *testing.T) {
	nodes := &stubEgressNodes{nodes: []domain.Node{
		unhealthyEgressNode(6, domain.ScopeWeb, 0, 5),
		healthyEgressNode(5, domain.ScopeWeb, 0, 0),
	}}
	accounts := &stubAccountBindings{accounts: map[accountdomain.Provider][]accountdomain.Credential{
		accountdomain.ProviderWeb: {
			activeWebAccount(40, accountdomain.EgressAssignmentAuto, 6, oldEnough()),
			activeWebAccount(41, accountdomain.EgressAssignmentAuto, 6, oldEnough()),
			activeWebAccount(42, accountdomain.EgressAssignmentAuto, 6, oldEnough()),
			activeWebAccount(43, accountdomain.EgressAssignmentAuto, 6, oldEnough()),
			activeWebAccount(44, accountdomain.EgressAssignmentAuto, 6, oldEnough()),
		},
	}}
	service := &Service{repository: nodes, accounts: accounts}
	service.ConfigureAutoAssignBounds(0, 0.5)

	result, err := service.RebalanceAccounts(context.Background(), true, false, time.Minute)
	if err != nil {
		t.Fatalf("RebalanceAccounts() error = %v", err)
	}
	if result.Rebalanced != 3 {
		t.Fatalf("RebalanceAccounts() = %+v, want 3 of 5 evacuated (ceil(5*0.5))", result)
	}
	if len(movedIDs(accounts.updates)) != 3 {
		t.Fatalf("moved ids = %v, want exactly 3", movedIDs(accounts.updates))
	}
}

func TestRebalanceAccountsNodeShareSpreadsEvacuation(t *testing.T) {
	nodes := &stubEgressNodes{nodes: []domain.Node{healthyEgressNode(1, domain.ScopeWeb, 10, 0)}}
	accounts := &stubAccountBindings{accounts: map[accountdomain.Provider][]accountdomain.Credential{
		accountdomain.ProviderWeb: {
			activeWebAccount(51, "", 0, nil),
			activeWebAccount(52, "", 0, nil),
			activeWebAccount(53, "", 0, nil),
			activeWebAccount(54, "", 0, nil),
			activeWebAccount(55, "", 0, nil),
			activeWebAccount(56, "", 0, nil),
			activeWebAccount(57, "", 0, nil),
			activeWebAccount(58, "", 0, nil),
			activeWebAccount(59, "", 0, nil),
			activeWebAccount(60, "", 0, nil),
		},
	}}
	service := &Service{repository: nodes, accounts: accounts}
	service.ConfigureAutoAssignBounds(0.5, 0)

	result, err := service.RebalanceAccounts(context.Background(), true, false, time.Minute)
	if err != nil {
		t.Fatalf("RebalanceAccounts() error = %v", err)
	}
	// A 0.5 node share caps the single healthy node at ceil(10*0.5)=5 accounts;
	// the overflow must stay unplaced instead of flooding the last node.
	if result.Assigned != 5 || result.Unplaced != 5 {
		t.Fatalf("RebalanceAccounts() = %+v, want 5 assigned and 5 unplaced", result)
	}
}

func TestRebalanceAccountsUnavailableOrInertWithoutAccounts(t *testing.T) {
	service := &Service{accounts: nil}
	if _, err := service.RebalanceAccounts(context.Background(), true, false, time.Minute); !errors.Is(err, ErrOperationsUnavailable) {
		t.Fatalf("error = %v, want ErrOperationsUnavailable", err)
	}
	// Both automatic modes disabled short-circuits without any movement.
	accounts := &stubAccountBindings{}
	service = &Service{repository: &stubEgressNodes{}, accounts: accounts}
	result, err := service.RebalanceAccounts(context.Background(), false, false, time.Minute)
	if err != nil || result != (RebalanceResult{}) {
		t.Fatalf("inert result = %+v, err = %v", result, err)
	}
	if len(accounts.updates) != 0 {
		t.Fatalf("unexpected updates = %+v", accounts.updates)
	}
}

func TestIsAutoAssignable(t *testing.T) {
	boundAuto := activeWebAccount(1, accountdomain.EgressAssignmentAuto, 5, nil)
	boundManual := activeWebAccount(2, accountdomain.EgressAssignmentManual, 5, nil)
	unbound := activeWebAccount(3, "", 0, nil)
	disabled := unbound
	disabled.Enabled = false
	reauth := unbound
	reauth.AuthStatus = accountdomain.AuthStatusReauthRequired

	tests := []struct {
		name        string
		credential  accountdomain.Credential
		autoAssign  bool
		autoBalance bool
		want        bool
	}{
		{name: "unbound requires autoAssign", credential: unbound, autoAssign: true, want: true},
		{name: "unbound inert without autoAssign", credential: unbound, autoBalance: true, want: false},
		{name: "bound auto repaired by either mode", credential: boundAuto, autoBalance: true, want: true},
		{name: "bound auto inert with both disabled", credential: boundAuto, want: false},
		{name: "bound auto repaired when only assigning", credential: boundAuto, autoAssign: true, want: true},
		{name: "manual never automatic", credential: boundManual, autoAssign: true, autoBalance: true, want: false},
		{name: "disabled never automatic", credential: disabled, autoAssign: true, want: false},
		{name: "reauth never automatic", credential: reauth, autoAssign: true, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isAutoAssignable(test.credential, test.autoAssign, test.autoBalance); got != test.want {
				t.Fatalf("isAutoAssignable() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestValidAutoAssignShare(t *testing.T) {
	for _, valid := range []float64{0, 0.05, 0.5, 1} {
		if !validAutoAssignShare(valid) {
			t.Fatalf("validAutoAssignShare(%v) = false", valid)
		}
	}
	for _, invalid := range []float64{0.01, 0.049, 1.5, -1, math.NaN(), math.Inf(1)} {
		if validAutoAssignShare(invalid) {
			t.Fatalf("validAutoAssignShare(%v) = true", invalid)
		}
	}
}

func containsID(ids []uint64, id uint64) bool {
	for _, value := range ids {
		if value == id {
			return true
		}
	}
	return false
}
