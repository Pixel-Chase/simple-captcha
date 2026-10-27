package tests

import (
	"crypto/md5"
	"fmt"
	"net/http"
)

// chromeDesktopUA 真实桌面 Chrome 的 UA；测试里追加唯一后缀隔离客户端状态
const chromeDesktopUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

// browserUA 生成带唯一后缀的桌面 Chrome UA，仍满足请求头核验对浏览器 UA 的判断
func browserUA(tag string) string {
	return chromeDesktopUA + " " + tag
}

// remoteAddrFor 由种子派生一个稳定的测试出口地址（198.18.0.0/15 为基准测试保留网段）。
// 同一种子（通常是带唯一后缀的 UA）得到同一地址，客户端绑定成立；不同用例、不同次运行互不共享出口 IP，
// 单 IP 加难计数不会跨用例累积而让断言默认难度的用例变得不稳定
func remoteAddrFor(seed string) string {
	sum := md5.Sum([]byte(seed))
	return fmt.Sprintf("198.18.%d.%d:4321", sum[0], 1+int(sum[1])%254)
}

// withBrowserHeaders 把请求头补成真实桌面 Chrome（Windows）发起跨域 fetch 的样子。
// 请求头核验上线后，不带这些头的请求会被加难；所有断言默认难度的用例都应经过这里
func withBrowserHeaders(req *http.Request, ua string) *http.Request {
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-CH-UA", `"Chromium";v="130", "Google Chrome";v="130", "Not?A_Brand";v="99"`)
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	return req
}
