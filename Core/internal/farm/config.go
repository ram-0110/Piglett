package farm

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	BlenderPath string `json:"blenderPath"`
	Version     string `json:"version"`
	Backend     string `json:"backend"`
	DeviceID    string `json:"deviceId"`
}

func configPath(dataDir string) string { return filepath.Join(dataDir, "core", "renderer.toml") }

func loadConfig(dataDir string) (Config, error) {
	data, err := os.Open(configPath(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	defer data.Close()
	var cfg Config
	scanner := bufio.NewScanner(data)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || line == "[blender]" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("invalid TOML line: %q", line)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key == "concurrency" {
			// Accept older renderer.toml files; scheduling now owns frame capacity.
			_, err = strconv.Atoi(value)
		} else {
			var decoded string
			decoded, err = strconv.Unquote(value)
			if err == nil {
				switch key {
				case "path":
					cfg.BlenderPath = decoded
				case "version":
					cfg.Version = decoded
				case "backend":
					cfg.Backend = decoded
				case "device_id":
					cfg.DeviceID = decoded
				default:
					return Config{}, fmt.Errorf("unknown TOML key: %s", key)
				}
			}
		}
		if err != nil {
			return Config{}, err
		}
	}
	return cfg, scanner.Err()
}

func saveConfig(dataDir string, cfg Config) error {
	path := configPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	body := fmt.Sprintf("[blender]\npath = %s\nversion = %s\nbackend = %s\ndevice_id = %s\n",
		strconv.Quote(cfg.BlenderPath), strconv.Quote(cfg.Version), strconv.Quote(cfg.Backend), strconv.Quote(cfg.DeviceID))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
