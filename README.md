# mosdns-cn

一个可以实现 本地/远程 DNS 分流的小工具。

- 自带缓存。
- 上游服务器支持 UDP/TCP/DoT/DoH 协议。
- 支持连接复用。可消除握手开销和延时。
- 支持通过 Socks5 连接上游服务器。
- 支持根据域名和 IP 分流。
- 支持常见的域名表格式。(一行一个域名)
- 支持常见的 IP 表格式。(一行一个 CIDR)
- 支持直接从 v2ray `dat` 文件载入域名和 IP 数据。
- 提供多种平台的预编译文件。无需配置，解压即用。

## 参数

```text
  -s, --server:           (必需) 监听地址。会同时监听 UDP 和 TCP。
                                 e.g. "127.0.0.1:53", ":53"    
  -c, --cache:            (可选) 缓存大小。单位: 条。(默认无缓存)
      --local-upstream:   (必需) 本地上游。这个参数可出现多次来配置多个上游。会并发请求所有上游。
      --local-ip:         (必需) 本地 IP 地址表。这个参数可出现多次，会从多个表载入数据。
      --local-domain:     (可选) 本地域名表。这些域名一定会被本地上游解析。这个参数可出现多次，会从多个表载入数据。
      --local-latency:    (可选) 本地上游延时，单位毫秒。(默认: 50)
      --remote-upstream:  (必需) 远程上游。这个参数可出现多次来配置多个上游。会并发请求所有上游。
      --remote-domain:    (可选) 远程域名表。这些域名一定会被远程上游解析。这个参数可出现多次，会从多个表载入数据。
  -v, --debug             Verbose log
```

上游支持 4 种协议。

- UDP: `8.8.8.8`, `208.67.222.222:443`
- TCP: `tcp://8.8.8.8`
- DoT: `tls://8.8.8.8`, `tls://dns.google`
- DoH: `https://8.8.8.8/dns-query`, `https://dns.google/dns-query`

还支持 3 个格外参数:

- `netaddr` 可以手动指定服务器实际网络地址(`ip:port`)。e.g. `tls://dns.google?netaddr=8.8.8.8:853`
- `socks5` 指定 socks5 代理服务器。UDP 协议暂不支持。e.g. `tls://dns.google?socks5=127.0.0.1:1080`
- `keepalive` 连接复用空连接保持时间。单位: 秒。不同协议默认值不同。DoH: 30，TCP/DoT: 0 (默认禁用连接复用)。
    - 启用连接复用后，只有第一个请求需要建立连接和握手，接下来的请求会在同一连接中直接传送。所以平均请求延时会和 UDP 一样低。
    - 不是什么黑科技，是 RFC 标准。
    - 绝大多数知名的公共 DNS 提供商都支持连接复用。比如 Cloudflare，Google，AliDNS。Google
      在其 [DoT 介绍页面](https://developers.google.com/speed/public-dns/docs/dns-over-tls#standards_support) 也表示"其 DoT 服务器支持
      RFC 7766 连接复用来实现高质量低延时的 DNS 服务"。
    - DoH 的连接复用会由 HTTP 自动协商，用户无需手动设置，已启用连接复用支持。
    - 但对于 TCP/DoT 协议，由于 RFC 7766 并没有强制要求服务器必须支持连接复用，因此这个选项协议默认禁用，需手动启用。你可以尝试开启 `keepalive`，然后用 `dig`
      之类的测试工具观察第一次请求和后续请求的延时变化，判断服务器是否支持连接复用。
    - 或者使用 mosdns 自带的探测服务器是否支持 TCP/DoT 连接复用的命令 `-probe-server-timeout`。
    - e.g. `tls://dns.google?keepalive=10`

- 如需同时使用多个参数，在地址后加 `?` 然后参数之间用 `&` 分隔
    - e.g. `tls://dns.google?netaddr=8.8.8.8:853&keepalive=10&socks5=127.0.0.1:1080`

域名表可以是:

- v2ray `geosite.dat` 文件。需用 `:` 指明类别。
- 文本文件。一个域名一行。默认子域名匹配，支持格外的匹配规则:
    - 以 `domain:` 开头或省略，子域名匹配。
    - 以 `keyword:` 开头，关键字匹配。
    - 以 `regexp:` 开头，正则匹配(Golang 标准)。
    - 以 `full:` 开头，完整匹配。

IP 表可以是:

- v2ray `geoip.dat` 文件。需用 `:` 指明类别。
- 文本文件。每行一个 IP 或 CIDR，支持 IPv6。

## 示例

```shell
mosdns-cn -s :53 -c 1024 --local-upstream https://223.5.5.5/dns-query --local-domain geosite.dat:cn --local-ip geoip.dat:cn --remote-upstream https://8.8.8.8/dns-query --remote-domain 'geosite.dat:geolocation-!cn'
```

## 相关连接

- 插件化 DNS 路由/转发器。[mosdns](https://github.com/IrineSistiana/mosdns)
- 常用域名/IP资源一步到位。[V2Ray 路由规则文件加强版](https://github.com/Loyalsoldier/v2ray-rules-dat)

## Open Source Components / Libraries / Reference

依赖

* [IrineSistiana/mosdns](https://github.com/IrineSistiana/mosdns): [GPL-3.0 License](https://github.com/IrineSistiana/mosdns/blob/main/LICENSE)
* [uber-go/zap](https://github.com/uber-go/zap): [LICENSE](https://github.com/uber-go/zap/blob/master/LICENSE.txt)
* [miekg/dns](https://github.com/miekg/dns): [LICENSE](https://github.com/miekg/dns/blob/master/LICENSE)
* [jessevdk/go-flags](https://github.com/jessevdk/go-flags): [BSD-3-Clause License](https://github.com/jessevdk/go-flags/blob/master/LICENSE)

---

Made with love ♥