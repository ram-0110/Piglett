package farm

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type BlenderInstallation struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

func installationsPath(dataDir string) string {
	return filepath.Join(dataDir, "core", "blenders.json")
}

func loadInstallations(dataDir string) ([]BlenderInstallation, error) {
	data, err := os.ReadFile(installationsPath(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return []BlenderInstallation{}, nil
	}
	if err != nil {
		return nil, err
	}
	var installations []BlenderInstallation
	if err := json.Unmarshal(data, &installations); err != nil {
		return nil, err
	}
	if installations == nil {
		installations = []BlenderInstallation{}
	}
	return installations, nil
}

func saveInstallations(dataDir string, installations []BlenderInstallation) error {
	path := installationsPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(installations, "", "  ")
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func upsertInstallation(installations []BlenderInstallation, entry BlenderInstallation) []BlenderInstallation {
	for i := range installations {
		if installations[i].Path == entry.Path {
			installations[i].Version = entry.Version
			return installations
		}
	}
	installations = append(installations, entry)
	sort.Slice(installations, func(i, j int) bool {
		if installations[i].Version == installations[j].Version {
			return installations[i].Path < installations[j].Path
		}
		return installations[i].Version > installations[j].Version
	})
	return installations
}
