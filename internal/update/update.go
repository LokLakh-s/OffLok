// Package update finds OffLok's releases on GitHub and downloads them, trusting nothing but what OffLok's release
// key signed: each release's SHA256SUMS, signed into SHA256SUMS.sig, names its files with their version and hash.
// The version offered is read from those signed names, never from GitHub's page, so an older release served again
// is no update.
package update

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultBase is where OffLok's releases are published.
var DefaultBase = "https://github.com/LokLakh-s/OffLok/releases"

// builtinKeys are the release keys OffLok trusts, by ID (the first 8 hex digits of the SHA-256 of the 32 public
// bytes). A new key is added in one release and the old one dropped in a later one.
var builtinKeys = map[string]string{
	"45e724ba": "ZRwR5kvfZ8i0mdz/rCo5dygv1HeJxw3r3RQ3IrfDS+U=",
}

// TrustedKeys returns the release keys built in.
func TrustedKeys() []ed25519.PublicKey {
	var out []ed25519.PublicKey
	for _, k := range builtinKeys {
		if pub, err := base64.StdEncoding.DecodeString(k); err == nil && len(pub) == ed25519.PublicKeySize {
			out = append(out, pub)
		}
	}
	return out
}

const (
	maxSums  = 64 << 10
	maxSig   = 1 << 10
	maxAsset = 512 << 20
)

// ErrSignature means SHA256SUMS does not carry a valid signature by a trusted key.
var ErrSignature = errors.New("the release's signature is not OffLok's")

// Asset is one file of a release.
type Asset struct {
	Name   string
	SHA256 string // hex
}

// Release is what a signed SHA256SUMS describes.
type Release struct {
	Version string
	Assets  []Asset
}

// Asset returns the file called name, if the release has it.
func (r Release) Asset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// Source fetches releases from Base.
type Source struct {
	Base      string
	Client    *http.Client
	UserAgent string
	Keys      []ed25519.PublicKey // nil: the keys built in (tests sign with their own)
}

func (s *Source) keys() []ed25519.PublicKey {
	if s.Keys != nil {
		return s.Keys
	}
	return TrustedKeys()
}

func (s *Source) base() string {
	if s.Base != "" {
		return strings.TrimSuffix(s.Base, "/")
	}
	return DefaultBase
}

// Page is the release's page, its notes.
func (s *Source) Page(version string) string { return s.base() + "/tag/v" + version }

func (s *Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func (s *Source) open(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if s.UserAgent != "" {
		req.Header.Set("User-Agent", s.UserAgent)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

func (s *Source) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	body, err := s.open(ctx, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = fmt.Errorf("%s: larger than %d bytes", url, limit)
	}
	return data, err
}

// Latest returns the newest release, checked against the release keys.
func (s *Source) Latest(ctx context.Context) (Release, error) {
	sums, err := s.get(ctx, s.base()+"/latest/download/SHA256SUMS", maxSums)
	if err != nil {
		return Release{}, err
	}
	sig, err := s.get(ctx, s.base()+"/latest/download/SHA256SUMS.sig", maxSig)
	if err != nil {
		return Release{}, err
	}
	return Parse(sums, sig, s.keys())
}

// Download fetches one file of r into dir and checks its hash; the file is only there once it matches.
func (s *Source) Download(ctx context.Context, r Release, name, dir string) (string, error) {
	a, ok := r.Asset(name)
	if !ok {
		return "", fmt.Errorf("release %s has no %s", r.Version, name)
	}
	body, err := s.open(ctx, s.base()+"/download/v"+r.Version+"/"+name)
	if err != nil {
		return "", err
	}
	defer body.Close()
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(body, maxAsset+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxAsset {
		err = fmt.Errorf("%s: larger than %d bytes", name, maxAsset)
	}
	if err == nil && hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		err = fmt.Errorf("%s: its SHA-256 is not the signed one", name)
	}
	if err != nil {
		os.Remove(path + ".part")
		return "", err
	}
	return path, os.Rename(path+".part", path)
}

// fileVersion reads the version in the name of OffLok's packages and installer: offlok_0.5.0_amd64.deb,
// offlok-desktop-0.5.0-1.x86_64.rpm, offlok-0.5.0-setup.exe.
var fileVersion = regexp.MustCompile(`^offlok(?:-desktop)?[_-](\d+\.\d+\.\d+)[_-]`)

// Parse checks sig over sums with keys and reads the release they describe.
func Parse(sums, sig []byte, keys []ed25519.PublicKey) (Release, error) {
	if !verify(sums, sig, keys) {
		return Release{}, ErrSignature
	}
	var r Release
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		hash, name, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if !ok {
			continue
		}
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if len(hash) != 64 || !isHex(hash) || name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
			return Release{}, fmt.Errorf("SHA256SUMS: unexpected line %q", sc.Text())
		}
		r.Assets = append(r.Assets, Asset{Name: name, SHA256: strings.ToLower(hash)})
		if m := fileVersion.FindStringSubmatch(name); m != nil {
			if r.Version != "" && r.Version != m[1] {
				return Release{}, fmt.Errorf("SHA256SUMS: two versions, %s and %s", r.Version, m[1])
			}
			r.Version = m[1]
		}
	}
	if r.Version == "" {
		return Release{}, errors.New("SHA256SUMS: no OffLok package in it")
	}
	return r, nil
}

func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}

func verify(msg, sig []byte, keys []ed25519.PublicKey) bool {
	if len(sig) != ed25519.SignatureSize {
		return false
	}
	for _, k := range keys {
		if len(k) == ed25519.PublicKeySize && ed25519.Verify(k, msg, sig) {
			return true
		}
	}
	return false
}

// Newer reports whether version v comes after current. A development build ("dev") is never updated.
func Newer(v, current string) bool {
	a, okA := parseVersion(v)
	b, okB := parseVersion(current)
	if !okA || !okB {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
