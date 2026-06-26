package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	dnsClassIN     = uint16(1)
	dnsTypeHTTPS   = uint16(65)
	dnsSVCParamECH = uint16(5)
)

// ECHConfigListResult holds the result of ECH config resolution.
type ECHConfigListResult struct {
	ConfigList []byte
	TTL        time.Duration
}

// resolveECHConfigList resolves ECH configuration via DoH for the given spec.
func resolveECHConfigList(ctx context.Context, spec ECHQuerySpec) (ECHConfigListResult, error) {
	query, err := buildDNSHTTPSQuery(spec.PublicName)
	if err != nil {
		return ECHConfigListResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spec.DoHURL, bytes.NewReader(query))
	if err != nil {
		return ECHConfigListResult{}, fmt.Errorf("build ECH DoH request: %w", err)
	}
	req.Header.Set("Accept", "application/dns-message")
	req.Header.Set("Content-Type", "application/dns-message")

	dohClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := dohClient.Do(req)
	if err != nil {
		return ECHConfigListResult{}, fmt.Errorf("query ECH config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ECHConfigListResult{}, fmt.Errorf("query ECH config: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ECHConfigListResult{}, fmt.Errorf("read ECH DoH response: %w", err)
	}

	ech, err := extractECHConfigListFromDNSMessage(body)
	if err == nil {
		return ech, nil
	}

	return resolveECHConfigListGET(ctx, spec, query)
}

func resolveECHConfigListGET(ctx context.Context, spec ECHQuerySpec, query []byte) (ECHConfigListResult, error) {
	u, err := url.Parse(spec.DoHURL)
	if err != nil {
		return ECHConfigListResult{}, fmt.Errorf("build ECH DoH GET request: %w", err)
	}
	q := u.Query()
	q.Set("dns", base64.RawURLEncoding.EncodeToString(query))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ECHConfigListResult{}, fmt.Errorf("build ECH DoH GET request: %w", err)
	}
	req.Header.Set("Accept", "application/dns-message")

	dohClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := dohClient.Do(req)
	if err != nil {
		return ECHConfigListResult{}, fmt.Errorf("query ECH config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ECHConfigListResult{}, fmt.Errorf("query ECH config: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ECHConfigListResult{}, fmt.Errorf("read ECH DoH response: %w", err)
	}
	return extractECHConfigListFromDNSMessage(body)
}

func buildDNSHTTPSQuery(name string) ([]byte, error) {
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[0:2], 0x1234)
	binary.BigEndian.PutUint16(msg[2:4], 0x0100)
	binary.BigEndian.PutUint16(msg[4:6], 1)

	var err error
	msg, err = appendDNSName(msg, name)
	if err != nil {
		return nil, err
	}
	msg = binary.BigEndian.AppendUint16(msg, dnsTypeHTTPS)
	msg = binary.BigEndian.AppendUint16(msg, dnsClassIN)
	return msg, nil
}

func appendDNSName(msg []byte, name string) ([]byte, error) {
	trimmed := strings.TrimSuffix(name, ".")
	if trimmed == "" {
		return append(msg, 0), nil
	}
	for _, label := range strings.Split(trimmed, ".") {
		if len(label) == 0 || len(label) > 63 {
			return nil, fmt.Errorf("invalid DNS name")
		}
		msg = append(msg, byte(len(label)))
		msg = append(msg, label...)
	}
	return append(msg, 0), nil
}

func extractECHConfigListFromDNSMessage(msg []byte) (ECHConfigListResult, error) {
	if len(msg) < 12 {
		return ECHConfigListResult{}, fmt.Errorf("invalid DNS response")
	}

	qdCount := int(binary.BigEndian.Uint16(msg[4:6]))
	anCount := int(binary.BigEndian.Uint16(msg[6:8]))
	offset := 12
	var err error
	for range qdCount {
		offset, err = skipDNSName(msg, offset)
		if err != nil {
			return ECHConfigListResult{}, err
		}
		if offset+4 > len(msg) {
			return ECHConfigListResult{}, fmt.Errorf("invalid DNS question")
		}
		offset += 4
	}

	for range anCount {
		offset, err = skipDNSName(msg, offset)
		if err != nil {
			return ECHConfigListResult{}, err
		}
		if offset+10 > len(msg) {
			return ECHConfigListResult{}, fmt.Errorf("invalid DNS answer")
		}
		rrType := binary.BigEndian.Uint16(msg[offset : offset+2])
		rrClass := binary.BigEndian.Uint16(msg[offset+2 : offset+4])
		rrTTL := time.Duration(binary.BigEndian.Uint32(msg[offset+4:offset+8])) * time.Second
		rdLength := int(binary.BigEndian.Uint16(msg[offset+8 : offset+10]))
		offset += 10
		if offset+rdLength > len(msg) {
			return ECHConfigListResult{}, fmt.Errorf("invalid DNS answer length")
		}
		rdataStart := offset
		rdataEnd := offset + rdLength
		offset = rdataEnd

		if rrType != dnsTypeHTTPS || rrClass != dnsClassIN {
			continue
		}
		ech, ok, err := extractECHFromHTTPSRData(msg, rdataStart, rdataEnd)
		if err != nil {
			return ECHConfigListResult{}, err
		}
		if ok {
			return ECHConfigListResult{ConfigList: ech, TTL: rrTTL}, nil
		}
	}

	return ECHConfigListResult{}, fmt.Errorf("missing ECH config")
}

func extractECHFromHTTPSRData(msg []byte, start int, end int) ([]byte, bool, error) {
	if start+2 > end {
		return nil, false, fmt.Errorf("invalid HTTPS record")
	}
	offset := start + 2
	next, err := skipDNSName(msg, offset)
	if err != nil {
		return nil, false, err
	}
	if next > end {
		return nil, false, fmt.Errorf("invalid HTTPS record")
	}
	offset = next

	for offset < end {
		if offset+4 > end {
			return nil, false, fmt.Errorf("invalid HTTPS parameter")
		}
		key := binary.BigEndian.Uint16(msg[offset : offset+2])
		length := int(binary.BigEndian.Uint16(msg[offset+2 : offset+4]))
		offset += 4
		if offset+length > end {
			return nil, false, fmt.Errorf("invalid HTTPS parameter length")
		}
		if key == dnsSVCParamECH {
			return append([]byte(nil), msg[offset:offset+length]...), true, nil
		}
		offset += length
	}
	return nil, false, nil
}

func skipDNSName(msg []byte, offset int) (int, error) {
	for {
		if offset >= len(msg) {
			return 0, fmt.Errorf("invalid DNS name")
		}
		length := int(msg[offset])
		offset++
		switch {
		case length == 0:
			return offset, nil
		case length&0xc0 == 0xc0:
			if offset >= len(msg) {
				return 0, fmt.Errorf("invalid DNS compression pointer")
			}
			return offset + 1, nil
		case length&0xc0 != 0:
			return 0, fmt.Errorf("invalid DNS label")
		default:
			offset += length
		}
	}
}

func parseECHQuerySpec(value string) (ECHQuerySpec, bool, error) {
	if value == "" {
		return ECHQuerySpec{}, false, nil
	}

	publicName, dohURL, found := strings.Cut(value, "+")
	if !found || publicName == "" || dohURL == "" {
		return ECHQuerySpec{}, false, fmt.Errorf("invalid ECH query")
	}
	u, err := url.Parse(dohURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ECHQuerySpec{}, false, fmt.Errorf("invalid ECH query")
	}

	return ECHQuerySpec{PublicName: publicName, DoHURL: dohURL}, true, nil
}
