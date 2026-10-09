package control

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func report(worker string, sequence uint64, policy string, state string) WorkerResilienceReport {
	return WorkerResilienceReport{
		SchemaVersion: "v1", WorkerID: worker, Sequence: sequence,
		Policies: []ResiliencePolicy{{Name: policy, Features: []string{"retry", "circuit_breaker"}}},
		Circuits: []ResilienceCircuit{{Name: policy, State: state}},
	}
}

func TestResilienceDiagnosticsBounds(t *testing.T) {
	store := NewResilienceDiagnosticsStore()
	now := time.Now()
	tooMany := report("worker", 1, "payments", "open")
	for i := 0; i < 256; i++ {
		name := fmt.Sprintf("policy-%d", i)
		tooMany.Policies = append(tooMany.Policies, ResiliencePolicy{Name: name})
	}
	if err := store.Record(tooMany, now); err == nil {
		t.Fatal("accepted 257 policies")
	}
	tooMany = report("worker", 1, "payments", "open")
	for i := 0; i < 256; i++ {
		name := fmt.Sprintf("policy-%d", i)
		tooMany.Policies = append(tooMany.Policies, ResiliencePolicy{Name: name})
		tooMany.Circuits = append(tooMany.Circuits, ResilienceCircuit{Name: name, State: "closed"})
	}
	if err := store.Record(tooMany, now); err == nil {
		t.Fatal("accepted 257 circuits")
	}
	for i := 0; i < 4096; i++ {
		if err := store.Record(WorkerResilienceReport{SchemaVersion: "v1", WorkerID: fmt.Sprintf("worker-%d", i), Sequence: 1}, now); err != nil {
			t.Fatalf("worker %d rejected: %v", i, err)
		}
	}
	if err := store.Record(WorkerResilienceReport{SchemaVersion: "v1", WorkerID: "overflow", Sequence: 1}, now); err == nil {
		t.Fatal("accepted worker beyond capacity")
	}
	if err := store.Record(WorkerResilienceReport{SchemaVersion: "v1", WorkerID: "worker-0", Sequence: 2}, now); err != nil {
		t.Fatalf("replacement at capacity rejected: %v", err)
	}
}

func TestResilienceDiagnosticsWorkerIsolationReplacementAndOrdering(t *testing.T) {
	store := NewResilienceDiagnosticsStore()
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for _, r := range []WorkerResilienceReport{report("worker-a", 1, "payments", "open"), report("worker-b", 1, "payments", "closed"), report("worker-a", 2, "payments", "half_open")} {
		if err := store.Record(r, now); err != nil {
			t.Fatal(err)
		}
	}
	for _, sequence := range []uint64{1, 2} {
		if err := store.Record(report("worker-a", sequence, "payments", "closed"), now); err == nil {
			t.Fatalf("accepted sequence %d", sequence)
		}
	}
	got := store.Snapshot(now, true)
	want := ResilienceWorkerCounts{Closed: 1, HalfOpen: 1}
	if len(got.Circuits) != 1 || got.Circuits[0].Workers != want {
		t.Fatalf("circuits = %#v, want %#v", got.Circuits, want)
	}
}

func TestResilienceDiagnosticsRejectsInvalidAtomically(t *testing.T) {
	store := NewResilienceDiagnosticsStore()
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	if err := store.Record(report("worker-a", 1, "payments", "open"), now); err != nil {
		t.Fatal(err)
	}
	invalid := []WorkerResilienceReport{
		report("worker-a", 2, "bad name", "closed"),
		report("worker-a", 2, "payments", "broken"),
		report("worker-a", 2, strings.Repeat("x", 129), "closed"),
		report("worker-a", 2, "payments", "closed"),
		report("worker-a", 2, "payments", "closed"),
	}
	invalid[3].Policies = append(invalid[3].Policies, invalid[3].Policies[0])
	invalid[4].SchemaVersion = "v2"
	for _, r := range invalid {
		if err := store.Record(r, now); err == nil {
			t.Fatalf("accepted invalid report: %#v", r)
		}
	}
	got := store.Snapshot(now, true)
	if len(got.Circuits) != 1 || got.Circuits[0].Workers.Open != 1 {
		t.Fatalf("invalid report changed stored state: %#v", got)
	}
}

func TestResilienceDiagnosticsDeterministicAggregateAndExpiry(t *testing.T) {
	store := NewResilienceDiagnosticsStore()
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	if got := store.Snapshot(now, true); got.Status != "unavailable" || got.ObservedAt != nil {
		t.Fatalf("empty snapshot = %#v", got)
	}
	first := report("z", 1, "zeta", "open")
	first.Policies[0].Features = []string{"retry", "circuit_breaker"}
	second := report("a", 1, "alpha", "closed")
	second.Policies[0].Features = []string{"timeout"}
	for _, r := range []WorkerResilienceReport{first, second} {
		if err := store.Record(r, now); err != nil {
			t.Fatal(err)
		}
	}
	first.Policies[0].Name = "mutated"
	got := store.Snapshot(now.Add(60*time.Second), true)
	if got.Status != "fresh" || got.ObservedAt == nil || !got.ObservedAt.Equal(now) {
		t.Fatalf("fresh snapshot = %#v", got)
	}
	if !reflect.DeepEqual(got.Policies, []ResiliencePolicy{{Name: "alpha", Features: []string{"timeout"}}, {Name: "zeta", Features: []string{"circuit_breaker", "retry"}}}) {
		t.Fatalf("policies = %#v", got.Policies)
	}
	if !reflect.DeepEqual(got.Circuits, []ResilienceCircuitSummary{{Name: "alpha", Workers: ResilienceWorkerCounts{Closed: 1}}, {Name: "zeta", Workers: ResilienceWorkerCounts{Open: 1}}}) {
		t.Fatalf("circuits = %#v", got.Circuits)
	}
	got.Policies[0].Features[0] = "mutated"
	if store.Snapshot(now, true).Policies[0].Features[0] != "timeout" {
		t.Fatal("snapshot shares stored memory")
	}
	stale := store.Snapshot(now.Add(60*time.Second+time.Nanosecond), true)
	if stale.Status != "stale" || stale.Circuits[0].Workers.Unknown != 1 || stale.Circuits[1].Workers.Unknown != 1 {
		t.Fatalf("stale snapshot = %#v", stale)
	}
	remote := store.Snapshot(now, false)
	if len(remote.Policies) != 0 || len(remote.Circuits) != 1 || remote.Circuits[0].Name != "" || remote.Circuits[0].Workers.Closed != 1 || remote.Circuits[0].Workers.Open != 1 {
		t.Fatalf("remote snapshot = %#v", remote)
	}
}
