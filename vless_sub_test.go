//go:build integration

package fetch

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type nodeResult struct {
	name    string
	proxyIP string
	exitIP  string
	latency time.Duration
	err     error
}

func TestVlessSubNodes(t *testing.T) {
	f, err := os.Open("/home/_/._/_/tool/vlesshttp/vless.sub")
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer f.Close()

	var nodes []struct {
		name    string
		uri     string
		proxyIP string
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "vless://") {
			continue
		}
		if !strings.Contains(line, "type=ws") {
			continue
		}
		hashIdx := strings.LastIndex(line, "#")
		name := ""
		uri := line
		if hashIdx > 0 {
			name = line[hashIdx+1:]
			uri = line[:hashIdx]
		}
		proxyIP := extractHost(uri)
		nodes = append(nodes, struct {
			name    string
			uri     string
			proxyIP string
		}{name: name, uri: uri, proxyIP: proxyIP})
	}

	t.Logf("共加载 %d 个 vless+ws 节点", len(nodes))

	results := make([]nodeResult, len(nodes))
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var failCount atomic.Int32

	sem := make(chan struct{}, 10)

	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, n struct {
			name    string
			uri     string
			proxyIP string
		}) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			res, err := Get(
				"https://cloudflare.com/cdn-cgi/trace",
				WithProxy(n.uri),
				WithTimeout(15*time.Second),
			)
			latency := time.Since(start)

			if err != nil {
				results[idx] = nodeResult{name: n.name, proxyIP: n.proxyIP, latency: latency, err: err}
				failCount.Add(1)
				return
			}

			exitIP := extractIPFromTrace(res.Text())
			results[idx] = nodeResult{name: n.name, proxyIP: n.proxyIP, exitIP: exitIP, latency: latency}
			if exitIP != "" {
				successCount.Add(1)
			} else {
				failCount.Add(1)
			}
		}(i, node)
	}

	wg.Wait()

	fmt.Println("\n| 节点名称 | 代理IP | 出口IP | 延迟 | 状态 |")
	fmt.Println("|----------|--------|--------|------|------|")
	for _, r := range results {
		status := "FAIL"
		exitIP := r.exitIP
		if r.err != nil {
			exitIP = r.err.Error()
		}
		if r.exitIP != "" && r.err == nil {
			status = "OK"
		}
		fmt.Printf("| %s | %s | %s | %dms | %s |\n",
			r.name, r.proxyIP, exitIP, r.latency.Milliseconds(), status)
	}

	fmt.Printf("\n总结: 总数=%d, 成功=%d, 失败=%d, 成功率=%.1f%%\n",
		len(nodes), successCount.Load(), failCount.Load(),
		float64(successCount.Load())/float64(len(nodes))*100)
}

func extractHost(uri string) string {
	afterScheme := strings.TrimPrefix(uri, "vless://")
	atIdx := strings.Index(afterScheme, "@")
	if atIdx < 0 {
		return ""
	}
	hostPort := afterScheme[atIdx+1:]
	colonIdx := strings.Index(hostPort, ":")
	if colonIdx < 0 {
		return hostPort
	}
	return hostPort[:colonIdx]
}

func extractIPFromTrace(traceResponse string) string {
	for _, line := range strings.Split(traceResponse, "\n") {
		if strings.HasPrefix(line, "ip=") {
			return strings.TrimPrefix(line, "ip=")
		}
	}
	return ""
}
