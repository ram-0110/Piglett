package farm

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxArchiveSize = 20 << 30
const maxExtractedSize = 100 << 30

func projectBlendFiles(root string) ([]string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, errors.New("choose a readable project folder")
	}
	var files []string
	var total int64
	count := 0
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("project contains a symbolic link: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("project contains an unsupported file: %s", path)
		}
		count++
		if count > 100000 {
			return errors.New("project contains too many files")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > maxExtractedSize {
			return errors.New("project folder is too large")
		}
		if strings.EqualFold(filepath.Ext(path), ".blend") {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, errors.New("this folder has no .blend files")
	}
	return files, nil
}

func packageProject(root, destination string) (string, error) {
	var err error
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if _, err := projectBlendFiles(root); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return "", err
	}
	temp := destination + ".tmp"
	output, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer os.Remove(temp)
	hash := sha256.New()
	limited := &limitWriter{writer: io.MultiWriter(output, hash), remaining: maxArchiveSize}
	archive := zip.NewWriter(limited)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported project file: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.Method = zip.Deflate
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err == nil {
		err = archive.Close()
	}
	closeErr := output.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Rename(temp, destination); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type limitWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *limitWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, errors.New("project archive exceeds 20 GB")
	}
	n, err := w.writer.Write(data)
	w.remaining -= int64(n)
	return n, err
}

func extractArchive(archive, destination string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	var total uint64
	for _, entry := range reader.File {
		name := filepath.Clean(filepath.FromSlash(entry.Name))
		if filepath.IsAbs(name) || name == "." || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) || entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe ZIP entry: %q", entry.Name)
		}
		if entry.UncompressedSize64 > maxExtractedSize-total {
			return errors.New("extracted project is too large")
		}
		total += entry.UncompressedSize64
		target := filepath.Join(destination, name)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
		if err != nil {
			source.Close()
			return err
		}
		_, err = io.CopyN(output, source, int64(entry.UncompressedSize64)+1)
		output.Close()
		source.Close()
		if err == nil {
			return fmt.Errorf("ZIP entry exceeds declared size: %q", entry.Name)
		}
		if !errors.Is(err, io.EOF) || size(target) != int64(entry.UncompressedSize64) {
			return fmt.Errorf("could not extract %q", entry.Name)
		}
	}
	return nil
}

func size(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

func safeBlendPath(root, relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || !strings.EqualFold(filepath.Ext(clean), ".blend") {
		return "", errors.New("choose a .blend file inside the archive")
	}
	path := filepath.Join(root, clean)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", errors.New("main .blend file was not found in the ZIP")
	}
	return path, nil
}
