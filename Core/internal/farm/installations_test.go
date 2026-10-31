package farm

import "testing"

func TestBlenderInstallationsPersistAlongsideActiveRenderer(t *testing.T) {
	dataDir := t.TempDir()
	active := Config{BlenderPath: "/apps/Blender-5.2", Version: "5.2.0", Backend: "CPU", DeviceID: "CPU"}
	if err := saveConfig(dataDir, active); err != nil {
		t.Fatal(err)
	}
	installations := upsertInstallation(nil, BlenderInstallation{Path: active.BlenderPath, Version: active.Version})
	installations = upsertInstallation(installations, BlenderInstallation{Path: "/apps/Blender-4.5", Version: "4.5.3"})
	installations = upsertInstallation(installations, BlenderInstallation{Path: active.BlenderPath, Version: active.Version})
	if err := saveInstallations(dataDir, installations); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadInstallations(dataDir)
	if err != nil || len(loaded) != 2 {
		t.Fatalf("saved Blender versions: %+v, %v", loaded, err)
	}
	cfg, err := loadConfig(dataDir)
	if err != nil || cfg.BlenderPath != active.BlenderPath {
		t.Fatalf("active renderer changed: %+v, %v", cfg, err)
	}
}

func TestExistingRendererAppearsInBlenderInstallations(t *testing.T) {
	dataDir := t.TempDir()
	if err := saveConfig(dataDir, Config{BlenderPath: "/apps/Blender-5.2", Version: "5.2.0", Backend: "CPU", DeviceID: "CPU"}); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	status := server.status()
	if len(status.Blenders) != 1 || status.Blenders[0].Version != "5.2.0" {
		t.Fatalf("existing Blender missing from settings: %+v", status.Blenders)
	}
}
