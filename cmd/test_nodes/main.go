package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/6643/fetch/proxy"
)

type cfg struct {
	raw string
	idx int
	tag string
}

type res struct {
	cfg cfg
	ok  bool
	err string
	ip  string
	tm  time.Duration
}

func main() {
	f, err := os.Open("/home/_/._/_/tool/sharedchat/vless.sub")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	var cfgs []cfg
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "vless://") {
			continue
		}
		tag := ""
		if idx := strings.LastIndex(line, "#"); idx >= 0 {
			tag = line[idx+1:]
		}
		cfgs = append(cfgs, cfg{raw: line, idx: len(cfgs) + 1, tag: tag})
	}
	if len(cfgs) > 20 {
		cfgs = cfgs[:20]
	}
	fmt.Printf("共 %d 个 VLESS 节点，取前 %d 个测试\n", len(cfgs), len(cfgs))

	var mu sync.Mutex
	var results []res
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for _, c := range cfgs {
		wg.Add(1)
		go func(c cfg) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := testNode(c)
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}(c)
	}
	wg.Wait()

	var pass, fail int
	for _, r := range results {
		if r.ok {
			pass++
			fmt.Printf("  ✅ [%3d] %-30s %5s  %s\n", r.cfg.idx, r.cfg.tag, r.tm.Round(time.Millisecond), r.ip)
		} else {
			fail++
			fmt.Printf("  ❌ [%3d] %-30s %s\n", r.cfg.idx, r.cfg.tag, r.err)
		}
	}
	fmt.Printf("\n通过 %d / %d\n", pass, len(results))
}

func testNode(c cfg) res {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t0 := time.Now()

	p, err := proxy.NewVlessProxy(c.raw)
	if err != nil {
		return res{cfg: c, err: fmt.Sprintf("NewVlessProxy: %v", err)}
	}
	if err := p.Start(ctx, ""); err != nil {
		return res{cfg: c, err: fmt.Sprintf("Start: %v", err)}
	}
	defer p.Close()

	proxyAddr := p.Addr().String()

	// 通过 HTTP 代理获取出口 IP
	transport := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			return url.Parse("http://" + proxyAddr)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second}

	resp, err := client.Get("https://ip.sb")
	if err != nil {
		return res{cfg: c, err: fmt.Sprintf("request: %v", err), tm: time.Since(t0)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	ip := strings.TrimSpace(string(body))
	if ip == "" {
		return res{cfg: c, err: "empty IP response", tm: time.Since(t0)}
	}

	return res{cfg: c, ok: true, ip: ip, tm: time.Since(t0)}
}
