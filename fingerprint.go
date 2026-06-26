// Package fetch — HTTP client library with proxy and TLS fingerprint support.
//
// TLS fingerprint functionality has been extracted to the tlsfingerprint
// submodule (github.com/6643/fetch/tlsfingerprint). These constants are
// re-exported here for backward compatibility.
package fetch

import (
	"github.com/6643/fetch/tlsfingerprint"
)

const (
	FingerprintChrome     = tlsfingerprint.FingerprintChrome
	FingerprintFirefox    = tlsfingerprint.FingerprintFirefox
	FingerprintSafari     = tlsfingerprint.FingerprintSafari
	FingerprintEdge       = tlsfingerprint.FingerprintEdge
	FingerprintIOS        = tlsfingerprint.FingerprintIOS
	FingerprintAndroid    = tlsfingerprint.FingerprintAndroid
	FingerprintRandom     = tlsfingerprint.FingerprintRandom
	FingerprintRandomized = tlsfingerprint.FingerprintRandomized
	FingerprintGolang     = tlsfingerprint.FingerprintGolang
	FingerprintCustom     = tlsfingerprint.FingerprintCustom
)
