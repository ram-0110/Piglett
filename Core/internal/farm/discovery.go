package farm

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const discoveryPort = 39852
const discoveryQuery = "PIGLETT_DISCOVER_1"

type discoveredPool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Link    string `json:"link"`
}

type discoveryReply struct {
	Protocol    int    `json:"protocol"`
	SessionID   string `json:"sessionId"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Port        int    `json:"port"`
	Fingerprint string `json:"fingerprint"`
}

func (s *Server) startDiscovery() {
	conn, err := net.ListenPacket("udp4", ":"+strconv.Itoa(discoveryPort))
	if err != nil {
		return
	} // Manual links remain available if another process owns the port.
	s.mu.Lock()
	s.discovery = conn
	s.mu.Unlock()
	go func() {
		defer conn.Close()
		buffer := make([]byte, 256)
		for {
			n, addr, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}
			if string(buffer[:n]) != discoveryQuery {
				continue
			}
			s.mu.Lock()
			var reply discoveryReply
			if s.network != nil && s.ledger != nil {
				reply = discoveryReply{Protocol: protocolVersion, SessionID: s.ledger.Job.ID, Name: s.ledger.Job.Name,
					Version: s.ledger.Job.Version, Port: s.networkPort, Fingerprint: s.fingerprint}
			}
			s.mu.Unlock()
			if reply.Port == 0 {
				continue
			}
			data, _ := json.Marshal(reply)
			_, _ = conn.WriteTo(data, addr)
		}
	}()
}

func (s *Server) localProjectFiles(w http.ResponseWriter, r *http.Request) {
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
	files, err := projectBlendFiles(request.Path)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"files": files})
}

func (s *Server) localDiscover(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	pools, err := discoverPools()
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"pools": pools})
}

func broadcastAddresses() []net.IP {
	addresses := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv4bcast}
	interfaces, _ := net.Interfaces()
	for _, item := range interfaces {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := item.Addrs()
		for _, address := range addrs {
			ipnet, ok := address.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			ip := ipnet.IP.To4()
			mask := ipnet.Mask
			if len(mask) != 4 {
				continue
			}
			addresses = append(addresses, net.IPv4(ip[0]|^mask[0], ip[1]|^mask[1], ip[2]|^mask[2], ip[3]|^mask[3]))
		}
	}
	return addresses
}

func discoverPools() ([]discoveredPool, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(1200 * time.Millisecond)); err != nil {
		return nil, err
	}
	for _, ip := range broadcastAddresses() {
		_, _ = conn.WriteToUDP([]byte(discoveryQuery), &net.UDPAddr{IP: ip, Port: discoveryPort})
	}
	pools := []discoveredPool{}
	seen := map[string]bool{}
	buffer := make([]byte, 2048)
	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				break
			}
			return nil, err
		}
		var reply discoveryReply
		if json.Unmarshal(buffer[:n], &reply) != nil || reply.Protocol != protocolVersion || reply.Port < 1 || reply.Port > 65535 || len(reply.Fingerprint) != 64 || reply.SessionID == "" || strings.Contains(reply.SessionID, "/") {
			continue
		}
		fingerprintOK := true
		for _, ch := range reply.Fingerprint {
			if !strings.ContainsRune("0123456789abcdef", ch) {
				fingerprintOK = false
				break
			}
		}
		if !fingerprintOK {
			continue
		}
		link := "https://" + net.JoinHostPort(addr.IP.String(), strconv.Itoa(reply.Port)) + "/session/" + url.PathEscape(reply.SessionID) + "#sha256=" + reply.Fingerprint
		if seen[link] {
			continue
		}
		seen[link] = true
		pools = append(pools, discoveredPool{Name: reply.Name, Version: reply.Version, Link: link})
	}
	sort.Slice(pools, func(i, j int) bool { return pools[i].Name < pools[j].Name })
	return pools, nil
}
