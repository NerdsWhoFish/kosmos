package operations

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"
)

func WithIntakeSigningKey(key []byte) ModuleOption {
	return func(m *Module) {
		m.intakeSigningKey = append([]byte(nil), key...)
		m.verifySignedIntake = true
	}
}

func clientIP(r *http.Request) (string, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return canonicalClientIP(host)
}

func canonicalClientIP(host string) (string, error) {
	address, err := netip.ParseAddr(host)
	if err != nil || address.Zone() != "" {
		return "", errors.New("invalid client address")
	}
	return address.Unmap().String(), nil
}

func signedClientIP(r *http.Request, key []byte, now time.Time) (string, error) {
	if len(key) < 32 {
		return "", errors.New("intake signing key is unavailable")
	}
	for _, name := range []string{"X-Kosmos-Client-IP", "X-Kosmos-Client-Time", "X-Kosmos-Client-Signature"} {
		if len(r.Header.Values(name)) != 1 {
			return "", errors.New("invalid signed client header")
		}
	}
	address, err := canonicalClientIP(r.Header.Get("X-Kosmos-Client-IP"))
	if err != nil || address != r.Header.Get("X-Kosmos-Client-IP") {
		return "", errors.New("invalid signed client address")
	}
	timestamp := r.Header.Get("X-Kosmos-Client-Time")
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || strconv.FormatInt(unix, 10) != timestamp || unix < now.Unix()-60 || unix > now.Unix()+60 {
		return "", errors.New("expired signed client address")
	}
	provided, err := hex.DecodeString(r.Header.Get("X-Kosmos-Client-Signature"))
	if err != nil || len(provided) != sha256.Size {
		return "", errors.New("invalid client signature")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("kosmos-intake-v1\n" + timestamp + "\n" + address))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return "", errors.New("invalid client signature")
	}
	return address, nil
}
