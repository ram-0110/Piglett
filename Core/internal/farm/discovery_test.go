package farm

import (
	"net/http"
	"strings"
	"testing"
)

func TestLocalPoolDiscovery(t *testing.T) {
	server := &Server{
		ledger:  &Ledger{Job: Job{ID: "session-test", Name: "sample", Version: "5.2.0"}},
		network: &http.Server{}, networkPort: 49123,
		fingerprint: strings.Repeat("a", 64),
	}
	server.startDiscovery()
	if server.discovery == nil {
		t.Skip("discovery UDP port is already in use")
	}
	defer server.discovery.Close()
	pools, err := discoverPools()
	if err != nil {
		t.Fatal(err)
	}
	for _, pool := range pools {
		if pool.Name == "sample" && pool.Version == "5.2.0" && strings.Contains(pool.Link, "/session/session-test#sha256=") {
			return
		}
	}
	t.Fatalf("local pool missing from discovery: %+v", pools)
}
