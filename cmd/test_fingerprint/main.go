package main

import (
	"fmt"
	"time"

	"github.com/6643/fetch"
)

func main() {
	url := "https://new.sharedchat.cc/frontend-api/getLoginConfig"

	fmt.Println("=== 测试1: 无指纹 (默认) ===")
	r1, err := fetch.Get(url, fetch.WithTimeout(10*time.Second))
	if err != nil {
		fmt.Printf("  ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("  ✅ 状态: %d, 长度: %d\n", r1.StatusCode, len(r1.Body))
	}

	fmt.Println("\n=== 测试2: Chrome TLS 指纹 ===")
	r2, err := fetch.Get(url,
		fetch.WithTimeout(10*time.Second),
		fetch.WithFingerprint(fetch.FingerprintChrome),
	)
	if err != nil {
		fmt.Printf("  ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("  ✅ 状态: %d, 长度: %d\n", r2.StatusCode, len(r2.Body))
	}

	fmt.Println("\n=== 测试3: Firefox TLS 指纹 ===")
	r3, err := fetch.Get(url,
		fetch.WithTimeout(10*time.Second),
		fetch.WithFingerprint(fetch.FingerprintFirefox),
	)
	if err != nil {
		fmt.Printf("  ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("  ✅ 状态: %d, 长度: %d\n", r3.StatusCode, len(r3.Body))
	}

	fmt.Println("\n=== 测试4: Edge TLS 指纹 ===")
	r4, err := fetch.Get(url,
		fetch.WithTimeout(10*time.Second),
		fetch.WithFingerprint(fetch.FingerprintEdge),
	)
	if err != nil {
		fmt.Printf("  ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("  ✅ 状态: %d, 长度: %d\n", r4.StatusCode, len(r4.Body))
	}

	fmt.Println("\n=== 测试5: Chrome 指纹 + POST 登录 ===")
	loginURL := "https://new.sharedchat.cc/frontend-api/login"
	r5, err := fetch.Post(loginURL,
		fetch.WithTimeout(10*time.Second),
		fetch.WithFingerprint(fetch.FingerprintChrome),
		fetch.WithJSON(map[string]string{
			"userToken": "if6643",
			"password":  "sharedchat",
		}),
	)
	if err != nil {
		fmt.Printf("  ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("  ✅ 状态: %d, 长度: %d\n", r5.StatusCode, len(r5.Body))
	}
}
