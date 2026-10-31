package farm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Server) NetworkHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/info", s.remoteInfo)
	mux.HandleFunc("/pair", s.remotePair)
	mux.HandleFunc("/archive", s.remoteArchive)
	mux.HandleFunc("/ready", s.remoteReady)
	mux.HandleFunc("/lease", s.remoteLease)
	mux.HandleFunc("/heartbeat", s.remoteHeartbeat)
	mux.HandleFunc("/result", s.remoteResult)
	mux.HandleFunc("/fail", s.remoteFail)
	return mux
}

func (s *Server) authorizedWorker(r *http.Request) (string, error) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	s.mu.Lock()
	id := s.tokens[token]
	s.mu.Unlock()
	if token == "" || id == "" {
		return "", errors.New("worker is not paired")
	}
	return id, nil
}

func (s *Server) remoteInfo(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	s.mu.Lock()
	ledger, hash := s.ledger, s.archiveHash
	s.mu.Unlock()
	if ledger == nil {
		writeError(w, 404, errors.New("no active job"))
		return
	}
	job, _ := ledger.Snapshot()
	writeJSON(w, 200, map[string]any{"protocol": protocolVersion, "sessionId": job.ID, "version": job.Version, "name": job.Name,
		"blendPath": job.BlendPath, "archiveHash": hash, "scene": job.Scene})
}

func (s *Server) remotePair(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var request struct {
		Code      string `json:"code"`
		SessionID string `json:"sessionId"`
		Name      string `json:"name"`
		Version   string `json:"version"`
		Backend   string `json:"backend"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	s.mu.Lock()
	code, ledger := s.pairCode, s.ledger
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	now := time.Now()
	attempts := s.pairAttempts[ip][:0]
	for _, attempt := range s.pairAttempts[ip] {
		if now.Sub(attempt) < time.Minute {
			attempts = append(attempts, attempt)
		}
	}
	attempts = append(attempts, now)
	s.pairAttempts[ip] = attempts
	limited := len(attempts) > 5
	expired := now.After(s.pairCodeExpires)
	s.mu.Unlock()
	if limited {
		writeError(w, 429, errors.New("too many pairing attempts; try later"))
		return
	}
	if expired {
		writeError(w, 403, errors.New("pairing code expired"))
		return
	}
	if ledger == nil || request.SessionID != ledger.Job.ID || request.Code != code {
		writeError(w, 403, errors.New("invalid session or pairing code"))
		return
	}
	worker, err := ledger.AddWorker(request.Name, request.Version, request.Backend)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	token := newID() + newID()
	s.mu.Lock()
	s.tokens[token] = worker.ID
	persistErr := s.saveHostState()
	s.mu.Unlock()
	if persistErr != nil {
		writeError(w, 500, persistErr)
		return
	}
	writeJSON(w, 200, map[string]string{"workerId": worker.ID, "token": token})
}

func (s *Server) remoteArchive(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	if _, err := s.authorizedWorker(r); err != nil {
		writeError(w, 401, err)
		return
	}
	s.mu.Lock()
	path := s.archivePath
	s.mu.Unlock()
	http.ServeFile(w, r, path)
}

func (s *Server) remoteReady(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	id, err := s.authorizedWorker(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	if err = s.ledger.Ready(id); err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ready": true})
}

func (s *Server) remoteLease(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	id, err := s.authorizedWorker(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	frame, err := s.ledger.Lease(id)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"frame": frame})
}

func (s *Server) remoteHeartbeat(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	id, err := s.authorizedWorker(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	var request struct {
		LeaseID string `json:"leaseId"`
	}
	if err = decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	if err = s.ledger.Heartbeat(id, request.LeaseID); err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) remoteFail(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	id, err := s.authorizedWorker(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	var request struct {
		LeaseID string `json:"leaseId"`
		Error   string `json:"error"`
	}
	if err = decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	if err = s.ledger.Fail(id, request.LeaseID, request.Error); err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) remoteResult(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	id, err := s.authorizedWorker(r)
	if err != nil {
		writeError(w, 401, err)
		return
	}
	leaseID := r.Header.Get("X-Piglett-Lease")
	frame, err := strconv.Atoi(r.Header.Get("X-Piglett-Frame"))
	if err != nil || frame < 0 || leaseID == "" {
		writeError(w, 400, errors.New("invalid result metadata"))
		return
	}
	if !s.ledger.ValidLease(id, leaseID, frame) {
		writeError(w, 409, errors.New("stale or unknown frame lease"))
		return
	}
	ext := strings.ToLower(r.Header.Get("X-Piglett-Extension"))
	if ext != ".png" && ext != ".exr" && ext != ".jpg" && ext != ".jpeg" && ext != ".tif" && ext != ".tiff" {
		writeError(w, 400, errors.New("unsupported output format"))
		return
	}
	job, _ := s.ledger.Snapshot()
	if err := os.MkdirAll(job.OutputPath, 0700); err != nil {
		writeError(w, 500, err)
		return
	}
	destination := filepath.Join(job.OutputPath, fmt.Sprintf("frame_%06d_%s%s", frame, leaseID[:min(len(leaseID), 8)], ext))
	temp := destination + ".part"
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(r.Body, (8<<30)+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || n > 8<<30 {
		_ = os.Remove(temp)
		writeError(w, 400, errors.New("invalid result upload"))
		return
	}
	if err := os.Rename(temp, destination); err != nil {
		writeError(w, 500, err)
		return
	}
	if err := s.ledger.Complete(id, leaseID, destination, hex.EncodeToString(hash.Sum(nil))); err != nil {
		_ = os.Remove(destination)
		writeError(w, 409, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"accepted": true})
}
