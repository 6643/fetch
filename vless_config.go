package fetch

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

var errInvalidConfig = errors.New("invalid vless config")

type vlessConfig struct {
	UUIDBytes      [16]byte
	ServerHost     string
	ServerPort     string
	TLSServerName  string
	TLSFingerprint string
	WebSocketHost  string
	WebSocketPath  string
	ECHSpec        *echQuerySpec
}

func parseVlessURI(raw string) (vlessConfig, error) {
	u, err := url.Parse(raw)
	if err != nil {
		if strings.Contains(err.Error(), "invalid port") {
			return vlessConfig{}, configMessage("invalid VLESS server port")
		}
		return vlessConfig{}, configMessage("malformed VLESS URI")
	}
	if !strings.EqualFold(u.Scheme, "vless") {
		return vlessConfig{}, configMessage("unsupported VLESS URI scheme")
	}

	uuidText := u.User.Username()
	uuidBytes, err := parseUUID(uuidText)
	if err != nil {
		return vlessConfig{}, configMessage("invalid VLESS UUID")
	}

	host := u.Hostname()
	if host == "" {
		return vlessConfig{}, configMessage("missing VLESS server host")
	}

	port := u.Port()
	if port == "" {
		return vlessConfig{}, configMessage("missing VLESS server port")
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return vlessConfig{}, configMessage("invalid VLESS server port")
	}

	q := u.Query()
	if security := strings.ToLower(q.Get("security")); security != "tls" {
		return vlessConfig{}, configMessage("unsupported VLESS security")
	}
	if transportType := strings.ToLower(q.Get("type")); transportType != "ws" {
		return vlessConfig{}, configMessage("unsupported VLESS transport type")
	}
	if encryption := strings.ToLower(q.Get("encryption")); encryption != "" && encryption != "none" {
		return vlessConfig{}, configMessage("unsupported VLESS encryption")
	}

	tlsServerName := q.Get("sni")
	if tlsServerName == "" {
		tlsServerName = host
	}
	tlsFingerprint := strings.ToLower(q.Get("fp"))
	if tlsFingerprint != "" && tlsFingerprint != "chrome" {
		return vlessConfig{}, configMessage("unsupported VLESS TLS fingerprint")
	}

	var echSpec *echQuerySpec
	if rawECH, found, err := rawQueryValue(u.RawQuery, "ech"); err != nil {
		return vlessConfig{}, configMessage("invalid ECH query")
	} else if found {
		spec, ok, err := parseECHQuerySpec(rawECH)
		if err != nil {
			return vlessConfig{}, configMessage(err.Error())
		}
		if ok {
			echSpec = &spec
		}
	}

	wsHost := q.Get("host")
	if wsHost == "" {
		wsHost = tlsServerName
	}
	wsPath := q.Get("path")
	if wsPath == "" {
		wsPath = "/"
	}
	if !strings.HasPrefix(wsPath, "/") {
		wsPath = "/" + wsPath
	}

	return vlessConfig{
		UUIDBytes:      uuidBytes,
		ServerHost:     host,
		ServerPort:     port,
		TLSServerName:  tlsServerName,
		TLSFingerprint: tlsFingerprint,
		WebSocketHost:  wsHost,
		WebSocketPath:  wsPath,
		ECHSpec:        echSpec,
	}, nil
}

func (c vlessConfig) serverAddr() string {
	return net.JoinHostPort(c.ServerHost, c.ServerPort)
}

func parseUUID(s string) ([16]byte, error) {
	var out [16]byte
	compact := strings.ReplaceAll(s, "-", "")
	if len(compact) != 32 {
		return out, fmt.Errorf("uuid must contain 16 bytes")
	}

	decoded, err := hex.DecodeString(compact)
	if err != nil {
		return out, err
	}
	copy(out[:], decoded)
	return out, nil
}

func configMessage(message string) error {
	return fmt.Errorf("%w: %s", errInvalidConfig, message)
}

func rawQueryValue(rawQuery string, targetKey string) (string, bool, error) {
	for _, pair := range strings.Split(rawQuery, "&") {
		if pair == "" {
			continue
		}

		key, value, _ := strings.Cut(pair, "=")
		decodedKey, err := url.PathUnescape(key)
		if err != nil {
			return "", false, err
		}
		if decodedKey != targetKey {
			continue
		}

		decodedValue, err := url.PathUnescape(value)
		if err != nil {
			return "", false, err
		}
		return decodedValue, true, nil
	}

	return "", false, nil
}
