// Package proxy provides VLESS proxy support with a local HTTP proxy server.
//
// Use DialContext to obtain a dial function for integration with http.Transport:
//
//	dialFn, err := proxy.DialContext("vless://uuid@host:port?security=tls&type=ws")
//	transport := &http.Transport{DialContext: dialFn}
//	client := &http.Client{Transport: transport}
//
// Use NewVlessProxy/Start to run a local HTTP proxy server that routes through VLESS:
//
//	p, err := proxy.NewVlessProxy("vless://uuid@host:port?...")
//	p.Start(ctx, "")
//	// Then configure curl -x http://127.0.0.1:<port>
//	defer p.Close()
package proxy
