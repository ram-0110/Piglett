package farm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyConcurrencyIsIgnoredAndRemovedOnSave(t *testing.T) {
	dataDir := t.TempDir()
	path := configPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := "[blender]\npath = \"/bin/blender\"\nversion = \"5.2.0\"\nbackend = \"CPU\"\ndevice_id = \"cpu\"\nconcurrency = 4\n"
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(dataDir)
	if err != nil || cfg.DeviceID != "cpu" {
		t.Fatalf("load legacy config: %+v, %v", cfg, err)
	}
	if err := saveConfig(dataDir, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "concurrency") {
		t.Fatalf("obsolete setting remained: %s", data)
	}
}
