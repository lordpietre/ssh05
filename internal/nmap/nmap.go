// Package nmap embeds a static Nmap binary and its data files for local
// network discovery.
//
// The binary and data are extracted once to the user cache. The binary is
// verified against its SHA-256 before execution. Linux amd64 and arm64 bundle
// a static binary; other platforms can still use an external --nmap binary.
package nmap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Version is the bundled Nmap release.
const Version = "7.99"

// Source documents where the static builds come from.
const Source = "https://github.com/whoisnian/static-binaries/releases/tag/v20260901.1"

//go:embed nmap-data.tar.gz
var nmapData []byte

const nmapDataSHA = "2cce6a39c645456d4a406069948a3f888dec122fc3eaa0ed1df967fbd1b58edf"

// ErrUnavailable means this platform has no embedded binary.
var ErrUnavailable = errors.New("nmap: no embedded binary for this platform")

var (
	extractOnce sync.Once
	extractPath string
	extractData string
	extractErr  error
)

// Binary returns the path to a verified, executable Nmap binary.
func Binary() (string, error) {
	extractOnce.Do(func() {
		extractPath, extractData, extractErr = extract()
	})
	return extractPath, extractErr
}

// DataDir returns the extracted Nmap data directory (services, probes,
// scripts and OS database).
func DataDir() (string, error) {
	extractOnce.Do(func() {
		extractPath, extractData, extractErr = extract()
	})
	return extractData, extractErr
}

func extract() (string, string, error) {
	compressed, platform, wantSHA := embeddedBinary()
	if len(compressed) == 0 {
		return "", "", fmt.Errorf("%w: %s/%s", ErrUnavailable, runtime.GOOS, runtime.GOARCH)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", "", fmt.Errorf("nmap: cache dir: %w", err)
	}
	dir := filepath.Join(cacheDir, "ssh05", "nmap", Version+"-"+platform)
	path := filepath.Join(dir, "nmap")
	dataDir := filepath.Join(dir, "share", "nmap")
	binaryReady := fileExists(path)
	dataReady := fileExists(filepath.Join(dataDir, "nmap-services"))
	if binaryReady && dataReady {
		return path, dataDir, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("nmap: create cache: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("nmap: secure cache: %w", err)
	}
	if !binaryReady {
		if err := extractBinary(compressed, dir, path, wantSHA); err != nil {
			return "", "", err
		}
	}
	if !dataReady {
		if err := extractDataFiles(dataDir); err != nil {
			return "", "", err
		}
	}
	return path, dataDir, nil
}

func extractBinary(compressed []byte, dir, path, wantSHA string) error {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("nmap: gzip: %w", err)
	}
	defer func() { _ = reader.Close() }()

	tmp, err := os.CreateTemp(dir, "nmap-*.tmp")
	if err != nil {
		return fmt.Errorf("nmap: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hash), reader); err != nil {
		cleanup()
		return fmt.Errorf("nmap: extract: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("nmap: close: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != wantSHA {
		cleanup()
		return fmt.Errorf("nmap: SHA-256 mismatch: got %s, want %s", actual, wantSHA)
	}
	if err := os.Chmod(tmpPath, 0o700); err != nil {
		cleanup()
		return fmt.Errorf("nmap: chmod: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("nmap: install: %w", err)
	}
	return nil
}

func extractDataFiles(dataDir string) error {
	sum := sha256.Sum256(nmapData)
	if actual := hex.EncodeToString(sum[:]); actual != nmapDataSHA {
		return fmt.Errorf("nmap: data SHA-256 mismatch: got %s", actual)
	}
	reader, err := gzip.NewReader(bytes.NewReader(nmapData))
	if err != nil {
		return fmt.Errorf("nmap: data gzip: %w", err)
	}
	defer func() { _ = reader.Close() }()
	archive := tar.NewReader(reader)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("nmap: data tar: %w", err)
		}
		name := filepath.Clean(header.Name)
		if name == "." || name == "" || filepath.IsAbs(name) || strings.HasPrefix(name, "..") {
			continue
		}
		target := filepath.Join(dataDir, name)
		if target != dataDir && !strings.HasPrefix(target, dataDir+string(os.PathSeparator)) {
			return fmt.Errorf("nmap: unsafe data path %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return fmt.Errorf("nmap: data dir: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return fmt.Errorf("nmap: data dir: %w", err)
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				return fmt.Errorf("nmap: data file: %w", err)
			}
			if _, err := io.Copy(file, archive); err != nil {
				_ = file.Close()
				return fmt.Errorf("nmap: data copy: %w", err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("nmap: data close: %w", err)
			}
		}
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}
