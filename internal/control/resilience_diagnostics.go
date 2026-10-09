package control

import (
	"errors"
	"sort"
	"sync"
	"time"
	"unicode/utf8"
)

const resilienceLease = 60 * time.Second
const maxResilienceEntries = 256
const maxResilienceWorkers = 4096
const maxResilienceNameBytes = 128

type WorkerResilienceReport struct {
	SchemaVersion string              `json:"schema_version"`
	WorkerID      string              `json:"worker_id"`
	Sequence      uint64              `json:"sequence"`
	Policies      []ResiliencePolicy  `json:"policies"`
	Circuits      []ResilienceCircuit `json:"circuits"`
}

type ResiliencePolicy struct {
	Name     string   `json:"name"`
	Features []string `json:"features"`
}

type ResilienceCircuit struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type ResilienceWorkerCounts struct {
	Closed   int `json:"closed"`
	Open     int `json:"open"`
	HalfOpen int `json:"half_open"`
	Unknown  int `json:"unknown"`
}

type ResilienceCircuitSummary struct {
	Name    string                 `json:"name,omitempty"`
	Workers ResilienceWorkerCounts `json:"workers"`
}

type ResilienceSummary struct {
	SchemaVersion string                     `json:"schema_version"`
	Status        string                     `json:"status"`
	ObservedAt    *time.Time                 `json:"observed_at,omitempty"`
	Policies      []ResiliencePolicy         `json:"policies"`
	Circuits      []ResilienceCircuitSummary `json:"circuits"`
}

type storedResilienceReport struct {
	report     WorkerResilienceReport
	receivedAt time.Time
}

type ResilienceDiagnosticsStore struct {
	mu      sync.RWMutex
	workers map[string]storedResilienceReport
}

func NewResilienceDiagnosticsStore() *ResilienceDiagnosticsStore {
	return &ResilienceDiagnosticsStore{workers: make(map[string]storedResilienceReport)}
}

func (s *ResilienceDiagnosticsStore) Record(report WorkerResilienceReport, receivedAt time.Time) error {
	if err := validateResilienceReport(report); err != nil {
		return err
	}
	copyOf := WorkerResilienceReport{
		SchemaVersion: report.SchemaVersion, WorkerID: report.WorkerID, Sequence: report.Sequence,
		Policies: make([]ResiliencePolicy, len(report.Policies)),
		Circuits: append([]ResilienceCircuit(nil), report.Circuits...),
	}
	for i, policy := range report.Policies {
		copyOf.Policies[i] = ResiliencePolicy{Name: policy.Name, Features: append([]string(nil), policy.Features...)}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, exists := s.workers[report.WorkerID]
	if exists && report.Sequence <= previous.report.Sequence {
		return errors.New("resilience sequence is not newer")
	}
	if !exists && len(s.workers) >= maxResilienceWorkers {
		return errors.New("resilience worker capacity reached")
	}
	s.workers[report.WorkerID] = storedResilienceReport{report: copyOf, receivedAt: receivedAt.UTC()}
	return nil
}

func validateResilienceReport(report WorkerResilienceReport) error {
	if report.SchemaVersion != "v1" || !validResilienceIdentifier(report.WorkerID) || report.Sequence == 0 || len(report.Policies) > maxResilienceEntries || len(report.Circuits) > maxResilienceEntries {
		return errors.New("invalid resilience report")
	}
	policies := make(map[string]bool, len(report.Policies))
	for _, policy := range report.Policies {
		if !validResilienceIdentifier(policy.Name) || policies[policy.Name] || len(policy.Features) > maxResilienceEntries {
			return errors.New("invalid resilience policy")
		}
		policies[policy.Name] = true
		features := make(map[string]bool, len(policy.Features))
		for _, feature := range policy.Features {
			if !validResilienceIdentifier(feature) || features[feature] {
				return errors.New("invalid resilience feature")
			}
			features[feature] = true
		}
	}
	circuits := make(map[string]bool, len(report.Circuits))
	for _, circuit := range report.Circuits {
		if !validResilienceIdentifier(circuit.Name) || circuits[circuit.Name] || !policies[circuit.Name] {
			return errors.New("invalid resilience circuit")
		}
		switch circuit.State {
		case "closed", "open", "half_open", "unknown":
		default:
			return errors.New("invalid resilience circuit state")
		}
		circuits[circuit.Name] = true
	}
	return nil
}

func validResilienceIdentifier(value string) bool {
	if len(value) == 0 || len(value) > maxResilienceNameBytes || !utf8.ValidString(value) {
		return false
	}
	for _, ch := range value {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.' {
			continue
		}
		return false
	}
	return true
}

func (s *ResilienceDiagnosticsStore) Snapshot(now time.Time, exposeNames bool) ResilienceSummary {
	result := ResilienceSummary{SchemaVersion: "v1", Status: "unavailable", Policies: []ResiliencePolicy{}, Circuits: []ResilienceCircuitSummary{}}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.workers) == 0 {
		return result
	}
	result.Status = "stale"
	policyFeatures := make(map[string]map[string]bool)
	circuitCounts := make(map[string]ResilienceWorkerCounts)
	var newest time.Time
	for _, worker := range s.workers {
		if worker.receivedAt.After(newest) {
			newest = worker.receivedAt
		}
		fresh := !now.After(worker.receivedAt.Add(resilienceLease))
		if fresh {
			result.Status = "fresh"
		}
		for _, policy := range worker.report.Policies {
			if policyFeatures[policy.Name] == nil {
				policyFeatures[policy.Name] = make(map[string]bool)
			}
			for _, feature := range policy.Features {
				policyFeatures[policy.Name][feature] = true
			}
		}
		for _, circuit := range worker.report.Circuits {
			counts := circuitCounts[circuit.Name]
			state := circuit.State
			if !fresh {
				state = "unknown"
			}
			switch state {
			case "closed":
				counts.Closed++
			case "open":
				counts.Open++
			case "half_open":
				counts.HalfOpen++
			default:
				counts.Unknown++
			}
			circuitCounts[circuit.Name] = counts
		}
	}
	observed := newest
	result.ObservedAt = &observed
	if !exposeNames {
		if len(circuitCounts) > 0 {
			var total ResilienceWorkerCounts
			for _, counts := range circuitCounts {
				total.Closed += counts.Closed
				total.Open += counts.Open
				total.HalfOpen += counts.HalfOpen
				total.Unknown += counts.Unknown
			}
			result.Circuits = append(result.Circuits, ResilienceCircuitSummary{Workers: total})
		}
		return result
	}
	for name, features := range policyFeatures {
		policy := ResiliencePolicy{Name: name, Features: make([]string, 0, len(features))}
		for feature := range features {
			policy.Features = append(policy.Features, feature)
		}
		sort.Strings(policy.Features)
		result.Policies = append(result.Policies, policy)
	}
	sort.Slice(result.Policies, func(i, j int) bool { return result.Policies[i].Name < result.Policies[j].Name })
	for name, counts := range circuitCounts {
		result.Circuits = append(result.Circuits, ResilienceCircuitSummary{Name: name, Workers: counts})
	}
	sort.Slice(result.Circuits, func(i, j int) bool { return result.Circuits[i].Name < result.Circuits[j].Name })
	return result
}

func (s *ResilienceDiagnosticsStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers = make(map[string]storedResilienceReport)
}
