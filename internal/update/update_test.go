package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

// Each built-in key is filed under its fingerprint: a typo in the key or its ID shows here.
func TestTrustedKeys(t *testing.T) {
	if len(TrustedKeys()) == 0 {
		t.Fatal("no release key built in")
	}
	for id, k := range builtinKeys {
		pub, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			t.Errorf("key %s: not an Ed25519 public key", id)
			continue
		}
		h := sha256.Sum256(pub)
		if id != "test" && hex.EncodeToString(h[:4]) != id {
			t.Errorf("key %s: its fingerprint is %x", id, h[:4])
		}
	}
}

func sumsOf(files map[string][]byte) []byte {
	var out []byte
	for name, data := range files {
		h := sha256.Sum256(data)
		out = append(out, fmt.Sprintf("%s  %s\n", hex.EncodeToString(h[:]), name)...)
	}
	return out
}

func TestParse(t *testing.T) {
	pub, priv := newKey(t)
	keys := []ed25519.PublicKey{pub}
	sums := []byte("" +
		"1111111111111111111111111111111111111111111111111111111111111111  offlok_0.5.0_amd64.deb\n" +
		"2222222222222222222222222222222222222222222222222222222222222222 *offlok-desktop-0.5.0-1.x86_64.rpm\n" +
		"3333333333333333333333333333333333333333333333333333333333333333  offlok-0.5.0-setup.exe\n" +
		"4444444444444444444444444444444444444444444444444444444444444444  offlok-release-45e724ba.pub.pem\n" +
		"5555555555555555555555555555555555555555555555555555555555555555  offlok-0.5.0.pkg\n" +
		"6666666666666666666666666666666666666666666666666666666666666666  offlok-0.5.0.apk\n")
	r, err := Parse(sums, ed25519.Sign(priv, sums), keys)
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "0.5.0" || len(r.Assets) != 6 {
		t.Fatalf("got %+v", r)
	}
	// The macOS package alone, as the update test serves it.
	pkg := []byte("5555555555555555555555555555555555555555555555555555555555555555  offlok-0.5.0.pkg\n")
	if r, err := Parse(pkg, ed25519.Sign(priv, pkg), keys); err != nil || r.Version != "0.5.0" {
		t.Fatalf("the macOS package alone: %+v %v", r, err)
	}
	if a, ok := r.Asset("offlok-desktop-0.5.0-1.x86_64.rpm"); !ok || a.SHA256[0] != '2' {
		t.Fatalf("the binary-mode line: %+v %v", a, ok)
	}

	tampered := append([]byte(nil), sums...)
	tampered[0] = '9'
	if _, err := Parse(tampered, ed25519.Sign(priv, sums), keys); !errors.Is(err, ErrSignature) {
		t.Fatalf("a changed SHA256SUMS: %v", err)
	}
	_, other, _ := ed25519.GenerateKey(nil)
	if _, err := Parse(sums, ed25519.Sign(other, sums), keys); !errors.Is(err, ErrSignature) {
		t.Fatalf("another key's signature: %v", err)
	}
	if _, err := Parse(sums, nil, keys); !errors.Is(err, ErrSignature) {
		t.Fatalf("no signature: %v", err)
	}

	for name, bad := range map[string]string{
		"two versions": "1111111111111111111111111111111111111111111111111111111111111111  offlok_0.5.0_amd64.deb\n" +
			"2222222222222222222222222222222222222222222222222222222222222222  offlok_0.4.0_arm64.deb\n",
		"no package":      "1111111111111111111111111111111111111111111111111111111111111111  notes.txt\n",
		"a .pkg not ours": "1111111111111111111111111111111111111111111111111111111111111111  offlok-0.5.0.pkg.txt\n",
		"another app's version": "1111111111111111111111111111111111111111111111111111111111111111  offlok_0.5.0_amd64.deb\n" +
			"2222222222222222222222222222222222222222222222222222222222222222  offlok-0.4.0.apk\n",
		"a path":     "1111111111111111111111111111111111111111111111111111111111111111  ../offlok_0.5.0_amd64.deb\n",
		"a bad hash": "11  offlok_0.5.0_amd64.deb\n",
	} {
		b := []byte(bad)
		if _, err := Parse(b, ed25519.Sign(priv, b), keys); err == nil || errors.Is(err, ErrSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		v, current string
		want       bool
	}{
		{"0.5.0", "0.4.0", true},
		{"0.10.0", "0.9.3", true},
		{"1.0.0", "0.99.99", true},
		{"0.4.0", "0.4.0", false},
		{"0.3.9", "0.4.0", false},
		{"0.5.0", "dev", false},
		{"0.5", "0.4.0", false},
	} {
		if got := Newer(c.v, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.v, c.current, got)
		}
	}
}

func TestSource(t *testing.T) {
	pub, priv := newKey(t)
	setup := []byte("the installer")
	sums := sumsOf(map[string][]byte{"offlok-0.5.0-setup.exe": setup})
	served := map[string][]byte{
		"/releases/latest/download/SHA256SUMS":             sums,
		"/releases/latest/download/SHA256SUMS.sig":         ed25519.Sign(priv, sums),
		"/releases/download/v0.5.0/offlok-0.5.0-setup.exe": setup,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "OffLok/test" {
			http.Error(w, "no user agent", http.StatusBadRequest)
			return
		}
		data, ok := served[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	defer srv.Close()
	s := &Source{Base: srv.URL + "/releases", UserAgent: "OffLok/test", Keys: []ed25519.PublicKey{pub}}
	ctx := context.Background()

	r, err := s.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "0.5.0" {
		t.Fatalf("version %q", r.Version)
	}
	dir := t.TempDir()
	path, err := s.Download(ctx, r, "offlok-0.5.0-setup.exe", dir)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "the installer" {
		t.Fatalf("downloaded %q", data)
	}

	// A file that is not the signed one never stays on disk.
	served["/releases/download/v0.5.0/offlok-0.5.0-setup.exe"] = []byte("something else")
	os.Remove(path)
	if _, err := s.Download(ctx, r, "offlok-0.5.0-setup.exe", dir); err == nil {
		t.Fatal("a changed file was accepted")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
	if _, err := s.Download(ctx, r, "offlok_0.5.0_amd64.deb", dir); err == nil {
		t.Fatal("a file the release does not list was fetched")
	}
}
