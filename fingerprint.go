// Package fetch — HTTP client library with proxy and TLS fingerprint support.
//
// TLS fingerprint functionality has been extracted to the fingerprint
// submodule (github.com/6643/fetch/fingerprint). These constants are
// re-exported here for backward compatibility.
package fetch

import (
	"github.com/6643/fetch/fingerprint"
)

const (
	FingerprintChrome     = fingerprint.FingerprintChrome
	FingerprintFirefox    = fingerprint.FingerprintFirefox
	FingerprintSafari     = fingerprint.FingerprintSafari
	FingerprintEdge       = fingerprint.FingerprintEdge
	FingerprintIOS        = fingerprint.FingerprintIOS
	FingerprintAndroid    = fingerprint.FingerprintAndroid
	FingerprintRandom     = fingerprint.FingerprintRandom
	FingerprintRandomized = fingerprint.FingerprintRandomized
	FingerprintGolang     = fingerprint.FingerprintGolang
	FingerprintCustom     = fingerprint.FingerprintCustom
)
