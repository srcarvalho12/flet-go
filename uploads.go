package flet

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"
)

const (
	// EnvMaxUploadSize sets WebOptions.MaxUploadSize, in bytes.
	EnvMaxUploadSize = "FLET_MAX_UPLOAD_SIZE"
	// EnvSecretKey sets WebOptions.SecretKey.
	EnvSecretKey = "FLET_SECRET_KEY"
)

// ErrUploadsDisabled is returned by Page.GetUploadURL when the app was not
// started with WebOptions.UploadDir.
var ErrUploadsDisabled = errors.New("flet: uploads are disabled (set WebOptions.UploadDir)")

// uploads receives files sent by FilePicker.Upload.
//
// An upload URL is {endpoint}?f={file}&e={unix expiry}&s={signature}, where
// the signature is an HMAC-SHA256 of the endpoint and the query, so a URL
// cannot be reused for another file or after it expires.
type uploads struct {
	dir      string
	endpoint string // absolute path, e.g. "/upload"
	maxSize  int64
	key      []byte
}

func newUploads(o WebOptions, endpoint string) (*uploads, error) {
	dir, err := filepath.Abs(o.UploadDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("flet: upload dir: %w", err)
	}
	u := &uploads{dir: dir, endpoint: endpoint, maxSize: o.MaxUploadSize, key: []byte(o.SecretKey)}
	if v := os.Getenv(EnvMaxUploadSize); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("flet: invalid %s: %q", EnvMaxUploadSize, v)
		}
		u.maxSize = n
	}
	if v := os.Getenv(EnvSecretKey); v != "" {
		u.key = []byte(v)
	}
	if len(u.key) == 0 {
		u.key = make([]byte, 32)
		rand.Read(u.key)
	}
	return u, nil
}

// url returns a signed URL that accepts one file for the given time.
func (u *uploads) url(name string, expires time.Duration) (string, error) {
	if _, err := u.clean(name); err != nil {
		return "", err
	}
	q := query(name, time.Now().Add(expires).Unix())
	return u.endpoint + "?" + q + "&s=" + u.sign(q), nil
}

func query(name string, expiry int64) string {
	return url.Values{"f": {name}, "e": {strconv.FormatInt(expiry, 10)}}.Encode()
}

func (u *uploads) sign(q string) string {
	mac := hmac.New(sha256.New, u.key)
	mac.Write([]byte(u.endpoint + "?" + q))
	return hex.EncodeToString(mac.Sum(nil))
}

// clean checks that name is a relative path that stays in the upload folder.
func (u *uploads) clean(name string) (string, error) {
	p := path.Clean("/" + filepath.ToSlash(name))
	if name == "" || p == "/" || filepath.IsAbs(name) || path.Clean(filepath.ToSlash(name)) != p[1:] {
		return "", fmt.Errorf("flet: invalid upload file name %q", name)
	}
	return filepath.FromSlash(p[1:]), nil
}

func (u *uploads) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	qv := r.URL.Query()
	name, exp, sig := qv.Get("f"), qv.Get("e"), qv.Get("s")
	expiry, err := strconv.ParseInt(exp, 10, 64)
	if name == "" || sig == "" || err != nil {
		http.Error(w, "missing upload parameters", http.StatusBadRequest)
		return
	}
	if !hmac.Equal([]byte(u.sign(query(name, expiry))), []byte(sig)) {
		http.Error(w, "invalid upload signature", http.StatusForbidden)
		return
	}
	if time.Now().Unix() >= expiry {
		http.Error(w, "upload URL has expired", http.StatusGone)
		return
	}
	rel, err := u.clean(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if u.maxSize > 0 && r.ContentLength > u.maxSize {
		http.Error(w, fmt.Sprintf("max upload size exceeded: %d bytes", u.maxSize), http.StatusRequestEntityTooLarge)
		return
	}

	// os.Root keeps every file operation inside the upload folder, even
	// through symbolic links.
	root, err := os.OpenRoot(u.dir)
	if err != nil {
		http.Error(w, "upload folder unavailable", http.StatusInternalServerError)
		return
	}
	defer root.Close()
	if d := filepath.Dir(rel); d != "." {
		if err := root.MkdirAll(d, 0o755); err != nil {
			http.Error(w, "invalid upload destination", http.StatusBadRequest)
			return
		}
	}
	tmp := rel + ".upload-" + randomID()
	f, err := root.Create(tmp)
	if err != nil {
		http.Error(w, "invalid upload destination", http.StatusBadRequest)
		return
	}
	body := io.Reader(r.Body)
	if u.maxSize > 0 {
		body = io.LimitReader(r.Body, u.maxSize+1)
	}
	n, err := io.Copy(f, body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
		root.Remove(tmp)
		http.Error(w, "upload failed", http.StatusBadRequest)
	case u.maxSize > 0 && n > u.maxSize:
		root.Remove(tmp)
		http.Error(w, fmt.Sprintf("max upload size exceeded: %d bytes", u.maxSize), http.StatusRequestEntityTooLarge)
	default:
		if err := root.Rename(tmp, rel); err != nil {
			root.Remove(tmp)
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// GetUploadURL returns a URL that accepts one upload of the file name
// (a path relative to WebOptions.UploadDir) for the given time. Pass it to
// FilePicker.Upload in a FilePickerUploadFile.
func (p *Page) GetUploadURL(name string, expires time.Duration) (string, error) {
	if p.uploads == nil {
		return "", ErrUploadsDisabled
	}
	return p.uploads.url(name, expires)
}
