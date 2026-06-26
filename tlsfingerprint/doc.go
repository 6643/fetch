// Package tlsfingerprint provides TLS fingerprinting (JA3/JA4) support using
// uTLS, enabling HTTP clients to mimic browser TLS handshakes.
//
// Supported fingerprints: chrome, firefox, safari, edge, ios, android,
// random, randomized, golang, custom.
//
// The Transport type wraps golang.org/x/net/http2 with a uTLS-based dialer,
// allowing HTTP/2 requests with custom browser fingerprints.
package tlsfingerprint
