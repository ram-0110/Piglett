package farm

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPoolReassignsDisconnectedWorkerFrame(t *testing.T) {
	ledger, err := NewLedger(filepath.Join(t.TempDir(), "job.json"), Job{
		Version: "5.2.0", Algorithm: "next", Scene: Scene{Start: 1, End: 3, Step: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := ledger.AddWorker("first", "5.2.0", "CPU")
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Ready(first.ID); err != nil {
		t.Fatal(err)
	}
	lease, err := ledger.Lease(first.ID)
	if err != nil || lease.Number != 1 {
		t.Fatalf("first lease: %+v, %v", lease, err)
	}
	second, err := ledger.AddWorker("second", "5.2.0", "CPU")
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Ready(second.ID); err != nil {
		t.Fatal(err)
	}
	ledger.mu.Lock()
	ledger.Job.Frames[0].Deadline = time.Now().Add(-time.Second)
	ledger.Workers[first.ID].LastSeen = time.Now().Add(-time.Minute)
	ledger.mu.Unlock()
	reassigned, err := ledger.Lease(second.ID)
	if err != nil || reassigned.Number != 1 || reassigned.LeaseID == lease.LeaseID {
		t.Fatalf("reassigned lease: %+v, %v", reassigned, err)
	}
	if err := ledger.Complete(first.ID, lease.LeaseID, "late.png", "hash"); err == nil {
		t.Fatal("late result was accepted")
	}
	if err := ledger.Complete(second.ID, reassigned.LeaseID, "frame.png", "hash"); err != nil {
		t.Fatal(err)
	}
	job, _ := ledger.Snapshot()
	if job.Frames[0].State != "completed" || job.Frames[0].ResultPath != "frame.png" {
		t.Fatalf("frame state: %+v", job.Frames[0])
	}
}

func TestVersionGateBeforeWorkerAdmission(t *testing.T) {
	ledger, err := NewLedger(filepath.Join(t.TempDir(), "job.json"), Job{
		Version: "5.2.0", Algorithm: "adaptive", Scene: Scene{Start: 1, End: 1, Step: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.AddWorker("old", "5.1.0", "CPU"); err == nil {
		t.Fatal("mismatched Blender version admitted")
	}
	if len(ledger.Workers) != 0 {
		t.Fatal("mismatched worker entered the pool")
	}
}

func TestEvenSpreadWaitsForOtherReadyWorker(t *testing.T) {
	ledger, err := NewLedger(filepath.Join(t.TempDir(), "job.json"), Job{
		Version: "5.2.0", Algorithm: "even", Scene: Scene{Start: 1, End: 4, Step: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := ledger.AddWorker("a", "5.2.0", "CPU")
	b, _ := ledger.AddWorker("b", "5.2.0", "CPU")
	_ = ledger.Ready(a.ID)
	_ = ledger.Ready(b.ID)
	first, err := ledger.Lease(a.ID)
	if err != nil || first == nil {
		t.Fatalf("first lease: %v", err)
	}
	if err := ledger.Complete(a.ID, first.LeaseID, "one.png", "one"); err != nil {
		t.Fatal(err)
	}
	if next, err := ledger.Lease(a.ID); err != nil || next != nil {
		t.Fatalf("even scheduler did not wait for second worker: %+v, %v", next, err)
	}
	second, err := ledger.Lease(b.ID)
	if err != nil || second == nil {
		t.Fatalf("second lease: %v", err)
	}
}

func TestWorkerReceivesOneFrameAtATime(t *testing.T) {
	ledger, err := NewLedger(filepath.Join(t.TempDir(), "job.json"), Job{
		Version: "5.2.0", Algorithm: "next", Scene: Scene{Start: 1, End: 3, Step: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := ledger.AddWorker("worker", "5.2.0", "CPU")
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Ready(worker.ID); err != nil {
		t.Fatal(err)
	}
	first, err := ledger.Lease(worker.ID)
	if err != nil || first == nil || first.Number != 1 {
		t.Fatalf("first lease: %+v, %v", first, err)
	}
	if second, err := ledger.Lease(worker.ID); err == nil || second != nil {
		t.Fatalf("worker received overlapping frame: %+v, %v", second, err)
	}
	if err := ledger.Complete(worker.ID, first.LeaseID, "one.png", "hash"); err != nil {
		t.Fatal(err)
	}
	second, err := ledger.Lease(worker.ID)
	if err != nil || second == nil || second.Number != 2 {
		t.Fatalf("next lease after completion: %+v, %v", second, err)
	}
}
