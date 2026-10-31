package farm

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Server struct {
	mu              sync.Mutex
	dataDir         string
	localToken      string
	config          Config
	installations   []BlenderInstallation
	caps            Capabilities
	ledger          *Ledger
	archivePath     string
	archiveHash     string
	discovery       net.PacketConn
	pairCode        string
	pairCodeExpires time.Time
	pairAttempts    map[string][]time.Time
	network         *http.Server
	networkURL      string
	networkPort     int
	certificate     tls.Certificate
	fingerprint     string
	tokens          map[string]string
	worker          *workerRuntime
}

const protocolVersion = 1

type Status struct {
	Config       Config                `json:"config"`
	Blenders     []BlenderInstallation `json:"blenders"`
	Capabilities Capabilities          `json:"capabilities"`
	SetupReady   bool                  `json:"setupReady"`
	Mode         string                `json:"mode"`
	Job          *Job                  `json:"job,omitempty"`
	Workers      []Worker              `json:"workers,omitempty"`
	Link         string                `json:"link,omitempty"`
	Code         string                `json:"code,omitempty"`
	Worker       *workerState          `json:"worker,omitempty"`
	Progress     *Progress             `json:"progress,omitempty"`
}

type Progress struct {
	Pending    int  `json:"pending"`
	Working    int  `json:"working"`
	Completed  int  `json:"completed"`
	Failed     int  `json:"failed"`
	ETASeconds *int `json:"etaSeconds,omitempty"`
}

func NewServer(dataDir string) (*Server, error) {
	cfg, err := loadConfig(dataDir)
	if err != nil {
		return nil, err
	}
	installations, err := loadInstallations(dataDir)
	if err != nil {
		return nil, err
	}
	if cfg.BlenderPath != "" {
		found := false
		for _, installation := range installations {
			if installation.Path == cfg.BlenderPath {
				found = true
				break
			}
		}
		if !found {
			installations = upsertInstallation(installations, BlenderInstallation{Path: cfg.BlenderPath, Version: cfg.Version})
			if err := saveInstallations(dataDir, installations); err != nil {
				return nil, err
			}
		}
	}
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return nil, err
	}
	s := &Server{dataDir: dataDir, config: cfg, installations: installations, localToken: hex.EncodeToString(key[:]), tokens: map[string]string{}, pairAttempts: map[string][]time.Time{}}
	s.certificate, s.fingerprint, err = loadOrCreateCertificate(dataDir)
	if err != nil {
		return nil, err
	}
	ledgerPath := filepath.Join(dataDir, "core", "active-job.json")
	if ledger, err := LoadLedger(ledgerPath); err == nil {
		s.ledger = ledger
		s.archivePath = filepath.Join(dataDir, "core", "jobs", ledger.Job.ID, "project.zip")
		s.archiveHash = ledger.Job.ArchiveHash
		var hostState struct {
			Port   int               `json:"port"`
			Tokens map[string]string `json:"tokens"`
		}
		if data, err := os.ReadFile(filepath.Join(dataDir, "core", "host-state.json")); err == nil {
			if json.Unmarshal(data, &hostState) == nil {
				s.networkPort = hostState.Port
				if hostState.Tokens != nil {
					s.tokens = hostState.Tokens
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("could not restore job ledger: %w", err)
	}
	return s, nil
}

func (s *Server) LocalToken() string { return s.localToken }

func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.worker != nil {
		s.worker.cancel()
	}
	if s.network != nil {
		_ = s.network.Close()
	}
	if s.discovery != nil {
		_ = s.discovery.Close()
	}
}

func (s *Server) status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := Status{Config: s.config, Blenders: append([]BlenderInstallation(nil), s.installations...), Capabilities: s.caps, SetupReady: s.config.BlenderPath != "", Mode: "idle"}
	if s.ledger != nil {
		job, workers := s.ledger.Snapshot()
		status.Job, status.Workers = &job, workers
		progress := &Progress{}
		for _, frame := range job.Frames {
			switch frame.State {
			case "pending":
				progress.Pending++
			case "working":
				progress.Working++
			case "completed":
				progress.Completed++
			case "failed":
				progress.Failed++
			}
		}
		var framesPerSecond float64
		for _, worker := range workers {
			if worker.Status != "offline" && worker.AvgSeconds > 0 {
				framesPerSecond += 1 / worker.AvgSeconds
			}
		}
		if framesPerSecond > 0 {
			seconds := int(float64(progress.Pending+progress.Working) / framesPerSecond)
			progress.ETASeconds = &seconds
		}
		status.Progress = progress
		status.Mode = "host-paused"
		if s.network != nil {
			if time.Now().After(s.pairCodeExpires) {
				s.pairCode = shortCode()
				s.pairCodeExpires = time.Now().Add(10 * time.Minute)
			}
			status.Mode, status.Link, status.Code = "host", s.networkURL, s.pairCode
		}
	}
	if s.worker != nil && s.network == nil {
		state := s.worker.snapshot()
		status.Mode, status.Worker = "worker", &state
	}
	return status
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func decodeJSON(r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(value)
}

func method(w http.ResponseWriter, r *http.Request, expected string) bool {
	if r.Method != expected {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return false
	}
	return true
}

func (s *Server) LocalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if method(w, r, http.MethodGet) {
			writeJSON(w, 200, s.status())
		}
	})
	mux.HandleFunc("/probe", s.localProbe)
	mux.HandleFunc("/blenders/add", s.localAddBlender)
	mux.HandleFunc("/project-files", s.localProjectFiles)
	mux.HandleFunc("/discover", s.localDiscover)
	mux.HandleFunc("/setup", s.localSetup)
	mux.HandleFunc("/host", s.localHost)
	mux.HandleFunc("/resume", s.localResume)
	mux.HandleFunc("/join", s.localJoin)
	mux.HandleFunc("/inspect", s.localInspect)
	mux.HandleFunc("/stop", s.localStop)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Piglett-Token") != s.localToken {
			writeError(w, 401, errors.New("unauthorized"))
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) localProbe(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var request struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	caps, err := ProbeBlender(request.Path)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, caps)
}

func (s *Server) localAddBlender(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var request struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	path, err := filepath.Abs(request.Path)
	if err != nil || request.Path == "" {
		writeError(w, 400, errors.New("choose a Blender application"))
		return
	}
	caps, err := ProbeBlender(path)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	s.mu.Lock()
	updated := upsertInstallation(append([]BlenderInstallation(nil), s.installations...), BlenderInstallation{Path: path, Version: caps.Version})
	err = saveInstallations(s.dataDir, updated)
	if err == nil {
		s.installations = updated
	}
	s.mu.Unlock()
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"installation": BlenderInstallation{Path: path, Version: caps.Version}, "capabilities": caps})
}

func (s *Server) localSetup(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	s.mu.Lock()
	active := s.network != nil || s.worker != nil
	s.mu.Unlock()
	if active {
		writeError(w, 409, errors.New("stop the active session before changing render device setup"))
		return
	}
	var request Config
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	if request.BlenderPath == "" {
		writeError(w, 400, errors.New("choose a Blender installation"))
		return
	}
	absolute, err := filepath.Abs(request.BlenderPath)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	request.BlenderPath = absolute
	caps, err := ProbeBlender(request.BlenderPath)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	found := false
	for _, device := range caps.Devices {
		if request.Backend == device.Backend && request.DeviceID == device.ID {
			found = true
		}
	}
	if !found {
		writeError(w, 400, errors.New("selected render device is unavailable"))
		return
	}
	request.Version = caps.Version
	s.mu.Lock()
	updated := upsertInstallation(append([]BlenderInstallation(nil), s.installations...), BlenderInstallation{Path: request.BlenderPath, Version: caps.Version})
	if err := saveInstallations(s.dataDir, updated); err != nil {
		s.mu.Unlock()
		writeError(w, 500, err)
		return
	}
	if err := saveConfig(s.dataDir, request); err != nil {
		s.mu.Unlock()
		writeError(w, 500, err)
		return
	}
	s.installations = updated
	s.config, s.caps = request, caps
	s.mu.Unlock()
	writeJSON(w, 200, s.status())
}

func (s *Server) setupConfig() (Config, error) {
	s.mu.Lock()
	cfg := s.config
	s.mu.Unlock()
	if cfg.BlenderPath == "" {
		return cfg, errors.New("complete Blender setup first")
	}
	caps, err := ProbeBlender(cfg.BlenderPath)
	if err != nil {
		return cfg, err
	}
	if caps.Version != cfg.Version {
		return cfg, errors.New("Blender version changed; repeat setup")
	}
	found := false
	for _, device := range caps.Devices {
		if device.Backend == cfg.Backend && device.ID == cfg.DeviceID {
			found = true
		}
	}
	if !found {
		return cfg, errors.New("render device changed; repeat setup")
	}
	s.mu.Lock()
	s.caps = caps
	s.mu.Unlock()
	return cfg, nil
}

func (s *Server) localHost(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	s.mu.Lock()
	active := s.network != nil || s.worker != nil
	s.mu.Unlock()
	if active {
		writeError(w, 409, errors.New("stop the active session before starting another job"))
		return
	}
	cfg, err := s.setupConfig()
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var request struct {
		ProjectPath string `json:"projectPath"`
		BlendPath   string `json:"blendPath"`
		Algorithm   string `json:"algorithm"`
		OutputPath  string `json:"outputPath"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	if request.Algorithm != "next" && request.Algorithm != "even" && request.Algorithm != "adaptive" {
		writeError(w, 400, errors.New("unknown scheduler"))
		return
	}
	if request.ProjectPath == "" || request.BlendPath == "" || request.OutputPath == "" {
		writeError(w, 400, errors.New("project folder, main .blend, and output folder are required"))
		return
	}
	files, err := projectBlendFiles(request.ProjectPath)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	selected := false
	for _, file := range files {
		if file == request.BlendPath {
			selected = true
			break
		}
	}
	if !selected {
		writeError(w, 400, errors.New("choose a .blend file from the project folder"))
		return
	}
	jobID := newID()
	jobDir := filepath.Join(s.dataDir, "core", "jobs", jobID)
	archivePath := filepath.Join(jobDir, "project.zip")
	hash, err := packageProject(request.ProjectPath, archivePath)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	projectRoot := filepath.Join(jobDir, "project")
	if err := extractArchive(archivePath, projectRoot); err != nil {
		writeError(w, 400, err)
		return
	}
	blend, err := safeBlendPath(projectRoot, request.BlendPath)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	scene, err := InspectBlend(cfg.BlenderPath, blend)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	job := Job{ID: jobID, Name: filepath.Base(filepath.Clean(request.ProjectPath)),
		BlendPath: request.BlendPath, Version: cfg.Version, Algorithm: request.Algorithm, Scene: scene,
		ArchiveHash: hash, OutputPath: request.OutputPath}
	ledger, err := NewLedger(filepath.Join(s.dataDir, "core", "active-job.json"), job)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	s.mu.Lock()
	if s.network != nil {
		_ = s.network.Close()
	}
	if s.worker != nil {
		s.worker.cancel()
		s.worker = nil
	}
	s.ledger, s.archivePath, s.archiveHash = ledger, archivePath, hash
	s.networkPort = 0
	s.tokens = map[string]string{}
	s.mu.Unlock()
	if err := s.startNetwork(); err != nil {
		writeError(w, 500, err)
		return
	}
	_ = os.MkdirAll(request.OutputPath, 0700)
	go s.startLocalWorker()
	writeJSON(w, 200, s.status())
}

func (s *Server) localResume(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	s.mu.Lock()
	active := s.network != nil || s.worker != nil
	s.mu.Unlock()
	if active {
		writeError(w, 409, errors.New("stop the active session before resuming this job"))
		return
	}
	cfg, err := s.setupConfig()
	if err != nil {
		writeError(w, 400, err)
		return
	}
	s.mu.Lock()
	hasJob := s.ledger != nil
	requiredVersion := ""
	if hasJob {
		requiredVersion = s.ledger.Job.Version
	}
	s.mu.Unlock()
	if !hasJob {
		writeError(w, 400, errors.New("no job to resume"))
		return
	}
	if cfg.Version != requiredVersion {
		writeError(w, 400, fmt.Errorf("saved job requires Blender %s; selected Blender is %s", requiredVersion, cfg.Version))
		return
	}
	if err := s.startNetwork(); err != nil {
		writeError(w, 500, err)
		return
	}
	go s.startLocalWorker()
	writeJSON(w, 200, s.status())
}

func (s *Server) localJoin(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	s.mu.Lock()
	active := s.network != nil || s.worker != nil
	s.mu.Unlock()
	if active {
		writeError(w, 409, errors.New("disconnect from the active session first"))
		return
	}
	cfg, err := s.setupConfig()
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var request struct {
		Link string `json:"link"`
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	if request.Link == "" || request.Code == "" {
		writeError(w, 400, errors.New("link and pairing code are required"))
		return
	}
	runtime, err := joinHost(s.dataDir, cfg, request.Link, request.Code)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	s.mu.Lock()
	if s.worker != nil {
		s.worker.cancel()
	}
	s.worker = runtime
	s.mu.Unlock()
	go runtime.run()
	writeJSON(w, 200, s.status())
}

func (s *Server) localInspect(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var request struct {
		Link string `json:"link"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, 400, err)
		return
	}
	info, _, err := inspectHost(request.Link)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, info)
}

func (s *Server) localStop(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	s.mu.Lock()
	if s.worker != nil {
		s.worker.cancel()
		s.worker = nil
	}
	if s.network != nil {
		_ = s.network.Close()
		s.network = nil
	}
	if s.discovery != nil {
		_ = s.discovery.Close()
		s.discovery = nil
	}
	s.mu.Unlock()
	writeJSON(w, 200, s.status())
}

func localIPv4() string {
	interfaces, _ := net.Interfaces()
	fallback := ""
	for _, item := range interfaces {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := item.Addrs()
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil && ip.IsPrivate() {
				if fallback == "" {
					fallback = ip.String()
				}
				name := strings.ToLower(item.Name)
				if strings.HasPrefix(name, "en") || strings.HasPrefix(name, "wl") || strings.Contains(name, "wi-fi") || strings.Contains(name, "ethernet") {
					return ip.String()
				}
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}

func shortCode() string {
	var digits [4]byte
	_, _ = rand.Read(digits[:])
	return fmt.Sprintf("%03d-%03d", (int(digits[0])<<8|int(digits[1]))%1000, (int(digits[2])<<8|int(digits[3]))%1000)
}

func (s *Server) startNetwork() error {
	s.mu.Lock()
	port := s.networkPort
	s.mu.Unlock()
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.network != nil {
		_ = s.network.Close()
	}
	s.pairCode = shortCode()
	s.pairCodeExpires = time.Now().Add(10 * time.Minute)
	s.networkPort = listener.Addr().(*net.TCPAddr).Port
	s.networkURL = fmt.Sprintf("https://%s:%d/session/%s#sha256=%s", localIPv4(), listener.Addr().(*net.TCPAddr).Port, s.ledger.Job.ID, s.fingerprint)
	s.network = &http.Server{Handler: s.NetworkHandler(), ReadHeaderTimeout: 5 * time.Second}
	server := s.network
	if err := s.saveHostState(); err != nil {
		_ = listener.Close()
		s.network = nil
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	tlsListener := tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{s.certificate}, MinVersion: tls.VersionTLS12})
	go func() { _ = server.Serve(tlsListener) }()
	s.startDiscovery()
	return nil
}

func (s *Server) saveHostState() error {
	path := filepath.Join(s.dataDir, "core", "host-state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{"port": s.networkPort, "tokens": s.tokens})
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
