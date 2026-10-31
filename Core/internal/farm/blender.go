package farm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Device struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Backend string `json:"backend"`
}

type Capabilities struct {
	Version string   `json:"version"`
	Cycles  bool     `json:"cycles"`
	Devices []Device `json:"devices"`
}

const probeScript = `import bpy, json
result = {"version": ".".join(map(str, bpy.app.version)), "cycles": bool(bpy.app.build_options.cycles), "devices": [{"id":"CPU","name":"CPU","type":"CPU","backend":"CPU"}]}
if result["cycles"]:
    prefs = bpy.context.preferences.addons["cycles"].preferences
    for backend in ["CUDA", "OPTIX", "HIP", "ONEAPI", "METAL"]:
        try:
            prefs.compute_device_type = backend
            prefs.refresh_devices()
            for device in prefs.devices:
                if device.type != "CPU":
                    result["devices"].append({"id":device.id,"name":device.name,"type":device.type,"backend":backend})
        except Exception:
            pass
print("PIGLETT_JSON:" + json.dumps(result))
`

const inspectScript = `import bpy, json
s = bpy.context.scene
result = {"engine":s.render.engine,"start":s.frame_start,"end":s.frame_end,"step":s.frame_step,"format":s.render.image_settings.file_format}
print("PIGLETT_JSON:" + json.dumps(result))
`

type Scene struct {
	Engine string `json:"engine"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Step   int    `json:"step"`
	Format string `json:"format"`
}

func runBlender(ctx context.Context, binary, blendFile, script string) ([]byte, error) {
	scriptFile, err := os.CreateTemp("", "piglett-*.py")
	if err != nil {
		return nil, err
	}
	defer os.Remove(scriptFile.Name())
	if _, err = scriptFile.WriteString(script); err != nil {
		scriptFile.Close()
		return nil, err
	}
	if err = scriptFile.Close(); err != nil {
		return nil, err
	}
	args := []string{"--background", "--factory-startup"}
	if blendFile != "" {
		args = append(args, blendFile)
	}
	args = append(args, "--python", scriptFile.Name())
	output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Blender failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	for _, line := range bytes.Split(output, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("PIGLETT_JSON:")) {
			return bytes.TrimPrefix(line, []byte("PIGLETT_JSON:")), nil
		}
	}
	return nil, errors.New("Blender did not return probe data")
}

func ProbeBlender(binary string) (Capabilities, error) {
	binary, err := filepath.Abs(binary)
	if err != nil {
		return Capabilities{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := runBlender(ctx, binary, "", probeScript)
	if err != nil {
		return Capabilities{}, err
	}
	var caps Capabilities
	err = json.Unmarshal(output, &caps)
	if err == nil && !caps.Cycles {
		err = errors.New("this Blender build does not support Cycles")
	}
	return caps, err
}

func InspectBlend(binary, path string) (Scene, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	output, err := runBlender(ctx, binary, path, inspectScript)
	if err != nil {
		return Scene{}, err
	}
	var scene Scene
	if err = json.Unmarshal(output, &scene); err != nil {
		return scene, err
	}
	if scene.Engine != "CYCLES" {
		return scene, fmt.Errorf("scene uses %s; Cycles is required", scene.Engine)
	}
	if scene.Step < 1 || scene.End < scene.Start {
		return scene, errors.New("invalid frame range")
	}
	switch scene.Format {
	case "PNG", "JPEG", "OPEN_EXR", "TIFF":
	default:
		return scene, fmt.Errorf("unsupported still-image output: %s", scene.Format)
	}
	return scene, nil
}
