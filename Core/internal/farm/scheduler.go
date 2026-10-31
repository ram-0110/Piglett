package farm

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const leaseDuration = 30 * time.Second
const reconnectGrace = 15 * time.Second
const maxAttempts = 3

type Frame struct {
	Number     int       `json:"number"`
	State      string    `json:"state"`
	WorkerID   string    `json:"workerId,omitempty"`
	LeaseID    string    `json:"leaseId,omitempty"`
	Deadline   time.Time `json:"deadline,omitempty"`
	Started    time.Time `json:"started,omitempty"`
	Attempts   int       `json:"attempts"`
	ResultPath string    `json:"resultPath,omitempty"`
	ResultHash string    `json:"resultHash,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type Worker struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	Backend    string    `json:"backend"`
	Status     string    `json:"status"`
	LastSeen   time.Time `json:"lastSeen"`
	Completed  int       `json:"completed"`
	Assigned   int       `json:"assigned"`
	AvgSeconds float64   `json:"avgSeconds"`
}

type Job struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	BlendPath   string  `json:"blendPath"`
	Version     string  `json:"version"`
	Algorithm   string  `json:"algorithm"`
	ArchiveHash string  `json:"archiveHash"`
	OutputPath  string  `json:"outputPath"`
	Scene       Scene   `json:"scene"`
	Frames      []Frame `json:"frames"`
}

type Ledger struct {
	mu      sync.Mutex
	path    string
	Job     Job
	Workers map[string]*Worker
}

func newID() string {
	var data [16]byte
	_, _ = rand.Read(data[:])
	return hex.EncodeToString(data[:])
}

func NewLedger(path string, job Job) (*Ledger, error) {
	ledger := &Ledger{path: path, Job: job, Workers: map[string]*Worker{}}
	if job.ID == "" {
		ledger.Job.ID = newID()
	}
	for frame := job.Scene.Start; frame <= job.Scene.End; frame += job.Scene.Step {
		ledger.Job.Frames = append(ledger.Job.Frames, Frame{Number: frame, State: "pending"})
		if len(ledger.Job.Frames) > 100000 {
			return nil, errors.New("frame range exceeds 100000 frames")
		}
	}
	return ledger, ledger.persist()
}

func LoadLedger(path string) (*Ledger, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ledger := &Ledger{path: path}
	if err := json.Unmarshal(data, ledger); err != nil {
		return nil, err
	}
	if ledger.Workers == nil {
		ledger.Workers = map[string]*Worker{}
	}
	for _, worker := range ledger.Workers {
		worker.Status = "offline"
	}
	return ledger, nil
}

func (l *Ledger) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Job     Job                `json:"job"`
		Workers map[string]*Worker `json:"workers"`
	}{l.Job, l.Workers})
}

func (l *Ledger) UnmarshalJSON(data []byte) error {
	var state struct {
		Job     Job                `json:"job"`
		Workers map[string]*Worker `json:"workers"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	l.Job, l.Workers = state.Job, state.Workers
	return nil
}

func (l *Ledger) persist() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	temp := l.path + ".tmp"
	if err := os.WriteFile(temp, data, 0600); err != nil {
		return err
	}
	return os.Rename(temp, l.path)
}

func (l *Ledger) AddWorker(name, version, backend string) (*Worker, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if version != l.Job.Version {
		return nil, fmt.Errorf("Blender %s is required; worker has %s", l.Job.Version, version)
	}
	worker := &Worker{ID: newID(), Name: name, Version: version, Backend: backend, Status: "transferring", LastSeen: time.Now()}
	l.Workers[worker.ID] = worker
	return worker, l.persist()
}

func (l *Ledger) Ready(workerID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	worker := l.Workers[workerID]
	if worker == nil {
		return errors.New("worker not found")
	}
	worker.Status, worker.LastSeen = "ready", time.Now()
	return l.persist()
}

func (l *Ledger) Heartbeat(workerID, leaseID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	worker := l.Workers[workerID]
	if worker == nil {
		return errors.New("worker not found")
	}
	worker.LastSeen = time.Now()
	worker.Status = "ready"
	if leaseID != "" {
		for i := range l.Job.Frames {
			frame := &l.Job.Frames[i]
			if frame.LeaseID == leaseID && frame.WorkerID == workerID && frame.State == "working" {
				frame.Deadline = time.Now().Add(leaseDuration)
			}
		}
	}
	for _, frame := range l.Job.Frames {
		if frame.State == "working" && frame.WorkerID == workerID {
			worker.Status = "busy"
			break
		}
	}
	return l.persist()
}

func (l *Ledger) expire(now time.Time) {
	for i := range l.Job.Frames {
		frame := &l.Job.Frames[i]
		if frame.State != "working" || now.Before(frame.Deadline) {
			continue
		}
		if worker := l.Workers[frame.WorkerID]; worker != nil && now.Sub(worker.LastSeen) < reconnectGrace {
			frame.Deadline = now.Add(reconnectGrace)
			continue
		}
		frame.State = "pending"
		if frame.Attempts >= maxAttempts {
			frame.State = "failed"
			frame.Error = "worker disconnected or lease expired"
		}
		frame.WorkerID, frame.LeaseID = "", ""
	}
	for _, worker := range l.Workers {
		if now.Sub(worker.LastSeen) > leaseDuration+reconnectGrace {
			worker.Status = "offline"
		}
	}
}

func (l *Ledger) nextFrame(workerID string) int {
	pending := make([]int, 0)
	for i, frame := range l.Job.Frames {
		if frame.State == "pending" {
			pending = append(pending, i)
		}
	}
	if len(pending) == 0 {
		return -1
	}
	if l.Job.Algorithm == "adaptive" && len(pending) > 1 {
		worker := l.Workers[workerID]
		if worker != nil && worker.AvgSeconds > 0 {
			var speedTotal float64
			var speedCount int
			for _, peer := range l.Workers {
				if peer.AvgSeconds > 0 && peer.Status != "offline" {
					speedTotal += peer.AvgSeconds
					speedCount++
				}
			}
			if speedCount > 1 {
				average := speedTotal / float64(speedCount)
				// Fast workers take estimated heavier frames. Use nearby completed
				// frame times as a cost hint; otherwise fall back to queue order.
				best, bestCost := pending[0], -1.0
				for _, index := range pending {
					var cost float64
					for _, done := range l.Job.Frames {
						if done.State == "completed" && done.Started.Unix() > 0 {
							distance := math.Abs(float64(done.Number - l.Job.Frames[index].Number))
							seconds := done.Deadline.Sub(done.Started).Seconds()
							if seconds > 0 {
								cost += seconds / (1 + distance)
							}
						}
					}
					if (worker.AvgSeconds < average && cost > bestCost) || (worker.AvgSeconds >= average && (bestCost < 0 || cost < bestCost)) {
						best, bestCost = index, cost
					}
				}
				return best
			}
		}
	}
	return pending[0]
}

func (l *Ledger) Lease(workerID string) (*Frame, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	worker := l.Workers[workerID]
	if worker == nil || (worker.Status != "ready" && worker.Status != "busy") {
		return nil, errors.New("worker is not ready")
	}
	now := time.Now()
	l.expire(now)
	for _, frame := range l.Job.Frames {
		if frame.WorkerID == workerID && frame.State == "working" {
			return nil, errors.New("worker is already rendering a frame")
		}
	}
	if l.Job.Algorithm == "even" {
		for _, peer := range l.Workers {
			if peer.Status != "offline" && peer.Status != "transferring" && worker.Assigned > peer.Assigned {
				return nil, nil
			}
		}
	}
	index := l.nextFrame(workerID)
	if index < 0 {
		return nil, nil
	}
	frame := &l.Job.Frames[index]
	frame.State, frame.WorkerID, frame.LeaseID = "working", workerID, newID()
	frame.Started, frame.Deadline = now, now.Add(leaseDuration)
	frame.Attempts++
	worker.Assigned++
	worker.Status, worker.LastSeen = "busy", now
	if err := l.persist(); err != nil {
		return nil, err
	}
	copy := *frame
	return &copy, nil
}

func (l *Ledger) Complete(workerID, leaseID, path, hash string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.Job.Frames {
		frame := &l.Job.Frames[i]
		if frame.LeaseID != leaseID || frame.WorkerID != workerID || frame.State != "working" {
			continue
		}
		now := time.Now()
		worker := l.Workers[workerID]
		duration := now.Sub(frame.Started).Seconds()
		if worker != nil {
			if worker.Completed == 0 {
				worker.AvgSeconds = duration
			} else {
				worker.AvgSeconds = 0.7*worker.AvgSeconds + 0.3*duration
			}
			worker.Completed++
			worker.Status, worker.LastSeen = "ready", now
			for _, other := range l.Job.Frames {
				if other.State == "working" && other.WorkerID == workerID {
					worker.Status = "busy"
					break
				}
			}
		}
		frame.State, frame.ResultPath, frame.ResultHash = "completed", path, hash
		frame.Deadline = now
		if worker != nil {
			worker.Status = "ready"
			for _, other := range l.Job.Frames {
				if other.State == "working" && other.WorkerID == workerID {
					worker.Status = "busy"
					break
				}
			}
		}
		return l.persist()
	}
	return errors.New("stale or unknown frame lease")
}

func (l *Ledger) ValidLease(workerID, leaseID string, number int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, frame := range l.Job.Frames {
		if frame.Number == number && frame.WorkerID == workerID && frame.LeaseID == leaseID && frame.State == "working" {
			return true
		}
	}
	return false
}

func (l *Ledger) Fail(workerID, leaseID, reason string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.Job.Frames {
		frame := &l.Job.Frames[i]
		if frame.LeaseID == leaseID && frame.WorkerID == workerID && frame.State == "working" {
			frame.State = "pending"
			if frame.Attempts >= maxAttempts {
				frame.State = "failed"
			}
			frame.Error, frame.LeaseID, frame.WorkerID = reason, "", ""
			if worker := l.Workers[workerID]; worker != nil {
				worker.Status = "ready"
				for _, other := range l.Job.Frames {
					if other.State == "working" && other.WorkerID == workerID {
						worker.Status = "busy"
						break
					}
				}
			}
			return l.persist()
		}
	}
	return errors.New("stale or unknown frame lease")
}

func (l *Ledger) Snapshot() (Job, []Worker) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.expire(time.Now())
	_ = l.persist()
	workers := make([]Worker, 0, len(l.Workers))
	for _, worker := range l.Workers {
		workers = append(workers, *worker)
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].Name < workers[j].Name })
	return l.Job, workers
}
