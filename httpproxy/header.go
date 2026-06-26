package httpproxy

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

const (
	vlessVersion           = byte(0x00)
	vlessCommandTCP        = byte(0x01)
	vlessAddressTypeIPv4   = byte(0x01)
	vlessAddressTypeDomain = byte(0x02)
	vlessAddressTypeIPv6   = byte(0x03)
)

// encodeVlessTCPRequestHeader builds the VLESS TCP request header for the given
// UUID and target address.
func encodeVlessTCPRequestHeader(uuidBytes [16]byte, targetAddr string) ([]byte, error) {
	host, portText, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, fmt.Errorf("split target address: %w", err)
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid target port")
	}

	header := make([]byte, 0, 64+len(host))
	header = append(header, vlessVersion)
	header = append(header, uuidBytes[:]...)
	header = append(header, 0x00)
	header = append(header, vlessCommandTCP)
	header = binary.BigEndian.AppendUint16(header, uint16(port))

	ip := net.ParseIP(host)
	switch {
	case ip.To4() != nil:
		header = append(header, vlessAddressTypeIPv4)
		header = append(header, ip.To4()...)
	case ip.To16() != nil:
		header = append(header, vlessAddressTypeIPv6)
		header = append(header, ip.To16()...)
	default:
		if len(host) == 0 || len(host) > 255 {
			return nil, fmt.Errorf("invalid target domain length")
		}
		header = append(header, vlessAddressTypeDomain, byte(len(host)))
		header = append(header, host...)
	}

	return header, nil
}
