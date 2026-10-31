package farm

import (
	"archive/zip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExtractRejectsTraversal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escape.blend")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractArchive(path, filepath.Join(t.TempDir(), "project")); err == nil {
		t.Fatal("traversal ZIP accepted")
	}
}

func TestProjectFolderFindsBlendFilesAndPackagesAssets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scenes"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{"scenes/main.blend": "blend", "scenes/alternate.blend": "alternate", "textures/color.png": "image"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := projectBlendFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files, []string{"scenes/alternate.blend", "scenes/main.blend"}) {
		t.Fatalf("blend files: %v", files)
	}
	archive := filepath.Join(t.TempDir(), "project.zip")
	if _, err := packageProject(root, archive); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got := map[string]bool{}
	for _, entry := range reader.File {
		got[entry.Name] = true
	}
	for _, name := range []string{"scenes/main.blend", "scenes/alternate.blend", "textures/color.png"} {
		if !got[name] {
			t.Fatalf("missing %s from archive", name)
		}
	}
}
