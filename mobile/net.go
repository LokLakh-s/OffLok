package mobile

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"time"
)

// listsClient downloads the block lists. The app is left out of its own tunnel, so that it sees the network the
// phone uses: its lookups go to OffLok's front on the loopback, as a computer's go to OffLok, and to the network's
// DNS only while OffLok is off.
func listsClient(front netip.AddrPort) *http.Client {
	offlok := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "udp", front.String())
	}}
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		ips, err := offlok.LookupNetIP(lctx, "ip", host)
		cancel()
		if err != nil {
			if ips, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host); err != nil {
				return nil, err
			}
		}
		var errs []error
		for _, ip := range ips {
			c, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if err == nil {
				return c, nil
			}
			errs = append(errs, err)
		}
		return nil, errors.Join(errs...)
	}
	return &http.Client{Timeout: time.Minute, Transport: &http.Transport{
		DialContext:         dial,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 15 * time.Second,
		IdleConnTimeout:     30 * time.Second,
	}}
}

// caDirs hold Android's certificate authorities, one PEM file each: the Conscrypt module's since Android 14, which
// Google Play updates, and the system's before.
var caDirs = []string{"/apex/com.android.conscrypt/cacerts", "/system/etc/security/cacerts"}

// writeCABundle writes Android's certificate authorities into one file, which Unbound checks DNS-over-TLS servers
// against (tls-cert-bundle). The certificates a user added are left out, as Android's apps leave them out by
// default.
func writeCABundle(path string) error {
	for _, dir := range caDirs {
		var out bytes.Buffer
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			for {
				var block *pem.Block
				block, data = pem.Decode(data)
				if block == nil {
					break
				}
				if block.Type == "CERTIFICATE" {
					_ = pem.Encode(&out, &pem.Block{Type: "CERTIFICATE", Bytes: block.Bytes})
				}
			}
		}
		if out.Len() == 0 {
			continue
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, out.Bytes(), 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	}
	return errors.New("no certificate authority found")
}
