package farm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type workerState struct {
	Host      string `json:"host"`
	JobName   string `json:"jobName"`
	Status    string `json:"status"`
	WorkerID  string `json:"workerId"`
	Completed int    `json:"completed"`
	Frames    []int  `json:"frames"`
	Error     string `json:"error,omitempty"`
}

type workerRuntime struct {
	mu          sync.Mutex
	state       workerState
	cfg         Config
	baseURL     string
	client      *http.Client
	fingerprint string
	token       string
	blendPath   string
	archiveHash string
	ctx         context.Context
	cancel      context.CancelFunc
}

type hostInfo struct {
	Protocol    int    `json:"protocol"`
	SessionID   string `json:"sessionId"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	BlendPath   string `json:"blendPath"`
	ArchiveHash string `json:"archiveHash"`
}

func inspectHost(link string) (hostInfo, *http.Client, error) {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || !strings.HasPrefix(parsed.Path, "/session/") {
		return hostInfo{}, nil, errors.New("enter a valid Piglett host link")
	}
	sessionID := strings.TrimPrefix(parsed.Path, "/session/")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		return hostInfo{}, nil, errors.New("invalid session link")
	}
	fingerprint := strings.TrimPrefix(parsed.Fragment, "sha256=")
	if len(fingerprint) != 64 || strings.ContainsAny(fingerprint, "/?&#") {
		return hostInfo{}, nil, errors.New("host link is missing its certificate fingerprint")
	}
	client := pinnedClient(fingerprint, 30*time.Second)
	var info hostInfo
	if err := requestJSON(client, "GET", parsed.Scheme+"://"+parsed.Host+"/info", "", nil, &info); err != nil {
		return hostInfo{}, nil, err
	}
	if info.SessionID != sessionID {
		return hostInfo{}, nil, errors.New("session link does not match host")
	}
	if info.Protocol != protocolVersion {
		return hostInfo{}, nil, errors.New("host and worker use incompatible Piglett Core versions")
	}
	return info, client, nil
}

func (w *workerRuntime) snapshot() workerState {
	w.mu.Lock()
	defer w.mu.Unlock()
	copy := w.state
	copy.Frames = append([]int(nil), w.state.Frames...)
	return copy
}

func (w *workerRuntime) update(fn func(*workerState)) {
	w.mu.Lock()
	fn(&w.state)
	w.mu.Unlock()
}

func requestJSON(client *http.Client, method, endpoint, token string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		if failure.Error == "" {
			failure.Error = resp.Status
		}
		return errors.New(failure.Error)
	}
	if output != nil {
		return json.NewDecoder(resp.Body).Decode(output)
	}
	return nil
}

func joinHost(dataDir string, cfg Config, link, code string) (*workerRuntime, error) {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || !strings.HasPrefix(parsed.Path, "/session/") {
		return nil, errors.New("enter a valid local host link")
	}
	sessionID := strings.TrimPrefix(parsed.Path, "/session/")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		return nil, errors.New("invalid session link")
	}
	base := parsed.Scheme + "://" + parsed.Host
	info, client, err := inspectHost(link)
	if err != nil {
		return nil, err
	}
	if info.Version != cfg.Version {
		return nil, fmt.Errorf("this job requires Blender %s; selected Blender is %s", info.Version, cfg.Version)
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = runtime.GOOS + " worker"
	}
	var pair struct {
		WorkerID string `json:"workerId"`
		Token    string `json:"token"`
	}
	err = requestJSON(client, "POST", base+"/pair", "", map[string]any{
		"code": code, "sessionId": sessionID, "name": hostname,
		"version": cfg.Version, "backend": cfg.Backend,
	}, &pair)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	worker := &workerRuntime{cfg: cfg, baseURL: base, token: pair.Token, archiveHash: info.ArchiveHash,
		client: client, fingerprint: strings.TrimPrefix(parsed.Fragment, "sha256="),
		ctx: ctx, cancel: cancel, state: workerState{Host: parsed.Host, JobName: info.Name, Status: "transferring", WorkerID: pair.WorkerID}}
	jobDir := filepath.Join(dataDir, "core", "worker-jobs", sessionID)
	archive := filepath.Join(jobDir, "project.zip")
	if err := os.MkdirAll(jobDir, 0700); err != nil {
		cancel()
		return nil, err
	}
	if !matchesHash(archive, info.ArchiveHash) {
		var transferErr error
		for attempt := 0; attempt < 3; attempt++ {
			transferErr = worker.downloadArchive(archive)
			if transferErr == nil {
				break
			}
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
		if transferErr != nil {
			cancel()
			return nil, transferErr
		}
	}
	projectRoot := filepath.Join(jobDir, "project")
	blend, err := safeBlendPath(projectRoot, info.BlendPath)
	if err != nil {
		if err := os.RemoveAll(projectRoot); err != nil {
			cancel()
			return nil, err
		}
		if err := extractArchive(archive, projectRoot); err != nil {
			cancel()
			return nil, err
		}
		blend, err = safeBlendPath(projectRoot, info.BlendPath)
		if err != nil {
			cancel()
			return nil, err
		}
	}
	worker.blendPath = blend
	return worker, nil
}

func matchesHash(path, expected string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == expected
}

func (w *workerRuntime) downloadArchive(destination string) error {
	req, err := http.NewRequestWithContext(w.ctx, "GET", w.baseURL+"/archive", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+w.token)
	transferClient := pinnedClient(w.fingerprint, 0)
	resp, err := transferClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("archive download: %s", resp.Status)
	}
	file, err := os.Create(destination + ".part")
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, maxArchiveSize+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || n > maxArchiveSize {
		_ = os.Remove(destination + ".part")
		return errors.New("archive transfer failed")
	}
	if hex.EncodeToString(hash.Sum(nil)) != w.archiveHash {
		_ = os.Remove(destination + ".part")
		return errors.New("archive checksum mismatch")
	}
	return os.Rename(destination+".part", destination)
}

func (w *workerRuntime) run() {
	for w.ctx.Err() == nil {
		err := requestJSON(w.client, "POST", w.baseURL+"/ready", w.token, map[string]bool{}, nil)
		if err == nil {
			break
		}
		w.update(func(s *workerState) { s.Status, s.Error = "reconnecting", err.Error() })
		select {
		case <-w.ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
	w.update(func(s *workerState) { s.Status = "ready" })
	w.slotLoop()
}

func (w *workerRuntime) slotLoop() {
	for w.ctx.Err() == nil {
		if err := requestJSON(w.client, "POST", w.baseURL+"/heartbeat", w.token,
			map[string]string{"leaseId": ""}, nil); err != nil {
			w.update(func(s *workerState) { s.Status, s.Error = "reconnecting", err.Error() })
			select {
			case <-w.ctx.Done():
				return
			case <-time.After(3 * time.Second):
				continue
			}
		}
		var result struct {
			Frame *Frame `json:"frame"`
		}
		err := requestJSON(w.client, "POST", w.baseURL+"/lease", w.token, map[string]bool{}, &result)
		if err != nil {
			w.update(func(s *workerState) { s.Status, s.Error = "reconnecting", err.Error() })
			select {
			case <-w.ctx.Done():
				return
			case <-time.After(3 * time.Second):
				continue
			}
		}
		if result.Frame == nil {
			w.update(func(s *workerState) {
				if len(s.Frames) == 0 {
					s.Status = "ready"
				}
				s.Error = ""
			})
			select {
			case <-w.ctx.Done():
				return
			case <-time.After(2 * time.Second):
				continue
			}
		}
		frame := result.Frame
		w.update(func(s *workerState) { s.Status = "busy"; s.Frames = append(s.Frames, frame.Number); s.Error = "" })
		w.renderAndUpload(frame)
		w.update(func(s *workerState) {
			for i, n := range s.Frames {
				if n == frame.Number {
					s.Frames = append(s.Frames[:i], s.Frames[i+1:]...)
					break
				}
			}
			if len(s.Frames) == 0 {
				s.Status = "ready"
			}
		})
	}
}

func (w *workerRuntime) renderAndUpload(frame *Frame) {
	ctx, cancel := context.WithTimeout(w.ctx, renderTimeout())
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = requestJSON(w.client, "POST", w.baseURL+"/heartbeat", w.token,
					map[string]string{"leaseId": frame.LeaseID}, nil)
			}
		}
	}()
	outputDir := filepath.Join(filepath.Dir(w.blendPath), ".piglett-output")
	output, err := renderFrame(ctx, w.cfg, w.blendPath, frame.Number, outputDir)
	if err != nil {
		cancel()
		<-heartbeatDone
		_ = requestJSON(w.client, "POST", w.baseURL+"/fail", w.token,
			map[string]string{"leaseId": frame.LeaseID, "error": err.Error()}, nil)
		w.update(func(s *workerState) { s.Error = err.Error() })
		return
	}
	for w.ctx.Err() == nil {
		err := w.uploadResult(frame, output)
		if err == nil || errors.Is(err, errStaleLease) {
			break
		}
		w.update(func(s *workerState) { s.Status, s.Error = "reconnecting", err.Error() })
		select {
		case <-w.ctx.Done():
			break
		case <-time.After(3 * time.Second):
		}
	}
	cancel()
	<-heartbeatDone
}

var errStaleLease = errors.New("frame lease is no longer active")

func (w *workerRuntime) uploadResult(frame *Frame, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	req, err := http.NewRequestWithContext(w.ctx, "POST", w.baseURL+"/result", file)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+w.token)
	req.Header.Set("X-Piglett-Lease", frame.LeaseID)
	req.Header.Set("X-Piglett-Frame", fmt.Sprint(frame.Number))
	req.Header.Set("X-Piglett-Extension", filepath.Ext(path))
	client := pinnedClient(w.fingerprint, 0)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return errStaleLease
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("result rejected: %s", resp.Status)
	}
	w.update(func(s *workerState) { s.Completed++ })
	return nil
}

func (s *Server) startLocalWorker() {
	s.mu.Lock()
	link, code, cfg := s.networkURL, s.pairCode, s.config
	s.mu.Unlock()
	worker, err := joinHost(s.dataDir, cfg, link, code)
	if err != nil {
		return
	}
	s.mu.Lock()
	if s.worker != nil {
		s.worker.cancel()
	}
	s.worker = worker
	s.mu.Unlock()
	worker.run()
}
