//go:build integration

package fetch

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var vlessNodes = []struct {
	name string
	uri  string
}{
	{"洛杉矶-01", "vless://19e9194d-e7c9-47ef-abe3-89071af826c2@198.41.196.39:443?encryption=none&security=tls&sni=abc.uni-faith.com&fp=chrome&type=ws&host=abc.uni-faith.com&path=%2F%3Fed%3D2048&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query"},
	{"洛杉矶-02", "vless://19e9194d-e7c9-47ef-abe3-89071af826c2@198.41.201.188:443?encryption=none&security=tls&sni=abc.uni-faith.com&fp=chrome&type=ws&host=abc.uni-faith.com&path=%2F%3Fed%3D2048&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query"},
	{"西雅图-01", "vless://19e9194d-e7c9-47ef-abe3-89071af826c2@104.17.250.207:443?encryption=none&security=tls&sni=abc.uni-faith.com&fp=chrome&type=ws&host=abc.uni-faith.com&path=%2F%3Fed%3D2048&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query"},
	{"华盛顿-01", "vless://19e9194d-e7c9-47ef-abe3-89071af826c2@103.31.4.215:443?encryption=none&security=tls&sni=abc.uni-faith.com&fp=chrome&type=ws&host=abc.uni-faith.com&path=%2F%3Fed%3D2048&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query"},
	{"洛杉矶-03", "vless://19e9194d-e7c9-47ef-abe3-89071af826c2@173.245.59.246:443?encryption=none&security=tls&sni=abc.uni-faith.com&fp=chrome&type=ws&host=abc.uni-faith.com&path=%2F%3Fed%3D2048&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query"},
	{"洛杉矶-04", "vless://19e9194d-e7c9-47ef-abe3-89071af826c2@162.159.235.134:443?encryption=none&security=tls&sni=abc.uni-faith.com&fp=chrome&type=ws&host=abc.uni-faith.com&path=%2F%3Fed%3D2048&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query"},
	{"洛杉矶-05", "vless://19e9194d-e7c9-47ef-abe3-89071af826c2@104.24.44.1:443?encryption=none&security=tls&sni=abc.uni-faith.com&fp=chrome&type=ws&host=abc.uni-faith.com&path=%2F%3Fed%3D2048&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query"},
	{"洛杉矶-06", "vless://19e9194d-e7c9-47ef-abe3-89071af826c2@172.67.92.70:443?encryption=none&security=tls&sni=abc.uni-faith.com&fp=chrome&type=ws&host=abc.uni-faith.com&path=%2F%3Fed%3D2048&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query"},
	{"洛杉矶-07", "vless://19e9194d-e7c9-47ef-abe3-89071af826c2@190.93.247.113:443?encryption=none&security=tls&sni=abc.uni-faith.com&fp=chrome&type=ws&host=abc.uni-faith.com&path=%2F%3Fed%3D2048&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query"},
	{"洛杉矶-08", "vless://19e9194d-e7c9-47ef-abe3-89071af826c2@190.93.247.61:443?encryption=none&security=tls&sni=abc.uni-faith.com&fp=chrome&type=ws&host=abc.uni-faith.com&path=%2F%3Fed%3D2048&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query"},
}

func TestVlessNodesGetIP(t *testing.T) {
	for _, node := range vlessNodes {
		t.Run(node.name, func(t *testing.T) {
			res, err := Get(
				"https://cloudflare.com/cdn-cgi/trace",
				WithVless(node.uri),
				WithTimeout(15*time.Second),
			)
			if err != nil {
				t.Fatalf("请求失败: %v", err)
			}

			if res.StatusCode != 200 {
				t.Fatalf("状态码异常: %d", res.StatusCode)
			}

			body := res.Text()
			ip := extractIP(body)
			if ip == "" {
				t.Fatalf("未找到IP地址")
			}

			t.Logf("节点: %s, IP: %s", node.name, ip)
			fmt.Printf("节点: %s, IP: %s\n", node.name, ip)
		})
	}
}

func extractIP(traceResponse string) string {
	for _, line := range strings.Split(traceResponse, "\n") {
		if strings.HasPrefix(line, "ip=") {
			return strings.TrimPrefix(line, "ip=")
		}
	}
	return ""
}
