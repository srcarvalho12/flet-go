package client

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

type entry struct {
	name, body, link string
	typ              byte
}

func makeTarGz(t *testing.T, entries []entry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.typ, Mode: 0o755, Linkname: e.link, Size: int64(len(e.body))}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if e.typ == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	gz.Close()
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractTarGz(t *testing.T) {
	archive := makeTarGz(t, []entry{
		{name: "Flet.app/", typ: tar.TypeDir},
		{name: "Flet.app/Contents/MacOS/Flet", body: "bin", typ: tar.TypeReg},
		{name: "Flet.app/Contents/Current", link: "MacOS", typ: tar.TypeSymlink},
	})
	dst := t.TempDir()
	if err := extractTarGz(archive, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dst, "Flet.app/Contents/MacOS/Flet"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Error("executable lost its exec permission")
	}
	if l, err := os.Readlink(filepath.Join(dst, "Flet.app/Contents/Current")); err != nil || l != "MacOS" {
		t.Errorf("wrong link: %q %v", l, err)
	}
	exe, err := executable(dst)
	if err != nil || filepath.Base(exe) != "Flet" {
		t.Errorf("executable() = %q, %v", exe, err)
	}
}

func TestExtractTarGzRejectsEscape(t *testing.T) {
	cases := [][]entry{
		{{name: "../outside", body: "x", typ: tar.TypeReg}},
		{{name: "link", link: "../../outside", typ: tar.TypeSymlink}},
		{{name: "link", link: "/etc/passwd", typ: tar.TypeSymlink}},
	}
	for i, c := range cases {
		if err := extractTarGz(makeTarGz(t, c), t.TempDir()); err == nil {
			t.Errorf("case %d: expected an error", i)
		}
	}
}
