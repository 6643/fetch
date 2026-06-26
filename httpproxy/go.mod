module github.com/6643/fetch/httpproxy

go 1.26.4

require (
	github.com/6643/fetch/tlsfingerprint v0.0.0
	github.com/coder/websocket v1.8.14
	github.com/refraction-networking/utls v1.8.3-0.20260301010127-aa6edf4b11af
)

require (
	github.com/andybalholm/brotli v1.0.6 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
)

replace github.com/6643/fetch/tlsfingerprint => ../tlsfingerprint
