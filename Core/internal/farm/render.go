package farm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const renderScript = `import bpy, sys
args = sys.argv[sys.argv.index("--") + 1:]
frame, output, backend, device_id = int(args[0]), args[1], args[2], args[3]
scene = bpy.context.scene
scene.render.engine = "CYCLES"
if backend == "CPU":
    scene.cycles.device = "CPU"
else:
    prefs = bpy.context.preferences.addons["cycles"].preferences
    prefs.compute_device_type = backend
    prefs.refresh_devices()
    found = False
    for device in prefs.devices:
        device.use = device.id == device_id
        found = found or device.use
    if not found:
        raise RuntimeError("Selected GPU is unavailable")
    scene.cycles.device = "GPU"
scene.frame_set(frame)
scene.render.filepath = output
bpy.ops.render.render(write_still=True)
print("PIGLETT_RENDER_DONE")
`

func renderFrame(ctx context.Context, cfg Config, blend string, frame int, outputDir string) (string, error) {
	if cfg.BlenderPath == "" {
		return "", errors.New("Blender setup is incomplete")
	}
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return "", err
	}
	script, err := os.CreateTemp("", "piglett-render-*.py")
	if err != nil {
		return "", err
	}
	defer os.Remove(script.Name())
	if _, err = script.WriteString(renderScript); err != nil {
		script.Close()
		return "", err
	}
	if err = script.Close(); err != nil {
		return "", err
	}
	prefix := filepath.Join(outputDir, fmt.Sprintf("frame_%06d", frame))
	command := exec.CommandContext(ctx, cfg.BlenderPath, "--background", "--factory-startup", blend, "--python", script.Name(), "--",
		strconv.Itoa(frame), prefix, cfg.Backend, cfg.DeviceID)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("frame %d failed: %w: %s", frame, err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(string(output), "PIGLETT_RENDER_DONE") {
		return "", errors.New("Blender did not confirm the render")
	}
	files, err := filepath.Glob(prefix + ".*")
	if err != nil || len(files) != 1 {
		return "", fmt.Errorf("expected one output for frame %d, found %d", frame, len(files))
	}
	return files[0], nil
}

func renderTimeout() time.Duration { return 12 * time.Hour }
