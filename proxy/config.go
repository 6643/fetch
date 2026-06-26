package proxy

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

// Config holds the parsed VLESS connection configuration.
type Config struct {
	UUIDBytes      [16]byte
	ServerHost     string
	ServerPort     string
	TLSServerName  string
	TLSFingerprint string
	WebSocketHost  string
	WebSocketPath  string
	ECHSpec        *ECHQuerySpec
}

// ServerAddr returns the host:port address of the VLESS server.
func (c Config) ServerAddr() string {
	return net.JoinHostPort(c.ServerHost, c.ServerPort)
}

// ECHQuerySpec holds parameters for resolving ECH (Encrypted Client Hello) config via DNS.
type ECHQuerySpec struct {
	PublicName string
	DoHURL     string
}

// ParseVlessURI parses a vless:// URI and returns the connection configuration.
func ParseVlessURI(raw string) (Config, error) {
	u, err := url.Parse(raw)
	if err != nil {
		if strings.Contains(err.Error(), "invalid port") {
			return Config{}, configMessage("invalid VLESS server port")
		}
		return Config{}, configMessage("malformed VLESS URI")
	}
	if !strings.EqualFold(u.Scheme, "vless") {
		return Config{}, configMessage("unsupported VLESS URI scheme")
	}

	uuidText := u.User.Username()
	uuidBytes, err := parseUUID(uuidText)
	if err != nil {
		return Config{}, configMessage("invalid VLESS UUID")
	}

	host := u.Hostname()
	if host == "" {
		return Config{}, configMessage("missing VLESS server host")
	}

	port := u.Port()
	if port == "" {
		return Config{}, configMessage("missing VLESS server port")
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return Config{}, configMessage("invalid VLESS server port")
	}

	q := u.Query()
	if security := strings.ToLower(q.Get("security")); security != "tls" {
		return Config{}, configMessage("unsupported VLESS security")
	}
	if transportType := strings.ToLower(q.Get("type")); transportType != "ws" {
		return Config{}, configMessage("unsupported VLESS transport type")
	}
	if encryption := strings.ToLower(q.Get("encryption")); encryption != "" && encryption != "none" {
		return Config{}, configMessage("unsupported VLESS encryption")
	}

	tlsServerName := q.Get("sni")
	if tlsServerName == "" {
		tlsServerName = host
	}
	tlsFingerprint := strings.ToLower(q.Get("fp"))
	if tlsFingerprint != "" && tlsFingerprint != "chrome" {
		return Config{}, configMessage("unsupported VLESS TLS fingerprint")
	}

	var echSpec *ECHQuerySpec
	if rawECH, found, err := rawQueryValue(u.RawQuery, "ech"); err != nil {
		return Config{}, configMessage("invalid ECH query")
	} else if found {
		spec, ok, err := parseECHQuerySpec(rawECH)
		if err != nil {
			return Config{}, configMessage(err.Error())
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

	return Config{
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
	for pair := range strings.SplitSeq(rawQuery, "&") {
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
