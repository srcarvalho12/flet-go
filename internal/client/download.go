package client

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Version is the Flet client version this SDK is compatible with.
const Version = "1.1.2"

// EnvURL overrides the client download URL (useful for tests and mirrors).
const EnvURL = "FLET_CLIENT_URL"

// artifact returns the release file name for the current OS.
func artifact() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "flet-macos.tar.gz", nil
	case "windows":
		return "flet-windows.zip", nil
	}
	return "", fmt.Errorf("automatic client download is not supported on %s yet; set %s", runtime.GOOS, EnvPath)
}

// cacheDir is where the extracted client is stored.
func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "flet-go", "client", "v"+Version), nil
}

// Resolve returns the path of the client executable.
// Order: FLET_CLIENT_PATH → local cache → download.
func Resolve() (string, error) {
	if p := os.Getenv(EnvPath); p != "" {
		return executable(p)
	}
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(dir); err == nil {
		return executable(dir)
	}
	if err := install(dir); err != nil {
		return "", err
	}
	return executable(dir)
}

// executable finds the executable inside a path (folder, .app or file).
func executable(p string) (string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("flet client not found at %s: %w", p, err)
	}
	if !info.IsDir() {
		return p, nil
	}
	if strings.HasSuffix(p, ".app") {
		return appExecutable(p)
	}
	switch runtime.GOOS {
	case "darwin":
		apps, _ := filepath.Glob(filepath.Join(p, "*.app"))
		if len(apps) == 0 {
			return "", fmt.Errorf("no .app found in %s", p)
		}
		return appExecutable(apps[0])
	case "windows":
		return filepath.Join(p, "flet", "flet.exe"), nil
	}
	return filepath.Join(p, "flet", "flet"), nil
}

func appExecutable(app string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(app, "Contents", "MacOS"))
	if err != nil || len(entries) == 0 {
		return "", fmt.Errorf("executable not found in %s", app)
	}
	return filepath.Join(app, "Contents", "MacOS", entries[0].Name()), nil
}

// install downloads and extracts the client into dir.
// It extracts into a temporary folder and renames it at the end,
// so a half-extracted cache is never left behind.
func install(dir string) error {
	name, err := artifact()
	if err != nil {
		return err
	}
	url := os.Getenv(EnvURL)
	if url == "" {
		url = fmt.Sprintf("https://github.com/flet-dev/flet/releases/download/v%s/%s", Version, name)
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Preparing Flet client v%s (first run only)...\n", Version)
	archive, err := download(url, filepath.Dir(dir))
	if err != nil {
		return err
	}
	defer os.Remove(archive)

	tmp := dir + "." + randomSuffix()
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	if strings.HasSuffix(name, ".zip") {
		err = extractZip(archive, tmp)
	} else {
		err = extractTarGz(archive, tmp)
	}
	if err != nil {
		os.RemoveAll(tmp)
		return fmt.Errorf("extract client: %w", err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		// Another process may have extracted it at the same time.
		if _, statErr := os.Stat(dir); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

func download(url, dir string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download client: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download client: %s returned %s", url, resp.Status)
	}

	f, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		return "", err
	}
	p := &progress{total: resp.ContentLength}
	_, err = io.Copy(f, io.TeeReader(resp.Body, p))
	fmt.Fprintln(os.Stderr)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("download client: %w", err)
	}
	return f.Name(), nil
}

// progress prints the download progress to the terminal.
type progress struct {
	total, done int64
	last        int64
}

func (p *progress) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.total > 0 {
		pct := p.done * 100 / p.total
		if pct != p.last {
			p.last = pct
			fmt.Fprintf(os.Stderr, "\rDownloading... %d%%", pct)
		}
	}
	return len(b), nil
}

// safeJoin prevents an archive entry from escaping the destination folder.
func safeJoin(dst, name string) (string, error) {
	p := filepath.Join(dst, name)
	if p != dst && !strings.HasPrefix(p, dst+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid path in archive: %s", name)
	}
	return p, nil
}

func extractTarGz(archive, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(dst, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(target, tr, h.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// The link must point inside the destination folder.
			if filepath.IsAbs(h.Linkname) {
				return fmt.Errorf("absolute link in archive: %s", h.Name)
			}
			if _, err := safeJoin(dst, filepath.Join(filepath.Dir(h.Name), h.Linkname)); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		}
	}
}

func extractZip(archive, dst string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		target, err := safeJoin(dst, zf.Name)
		if err != nil {
			return err
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		err = writeFile(target, rc, zf.Mode().Perm()|0o600)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeFile(target string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func randomSuffix() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
