# mosdns-cn

极简配置。无需格外的配置文件。

支持缓存。支持 UDP/TCP/DoT/DoH 协议。支持 Socks5。支持域名分流。支持从 v2ray `dat` 文件载入数据。

## 参数

```text
  -s, --server:           (必需) 监听地址。会同时监听 UDP 和 TCP。
                          e.g. "127.0.0.1:53", ":53"    
  -c, --cache:            (可选) 缓存大小。
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

可以添加 `netaddr` 参数手动指定服务器实际网络地址(`ip:port`)。e.g. `tcp://dns.google?netaddr=8.8.8.8:853`

mosdns-cn 不支持 bootstrap。所以如果系统自身的 DNS 服务器是 mosdns-cn，**必须**手动指定服务器的网络地址。

TCP/DoT/DoH 协议支持可以添加 `socks5` 参数通过 Socks5 代理连接服务器。e.g. `tcp://dns.google?socks5=127.0.0.1:1080`

域名表可以是:

- v2ray `geosite.dat` 文件。需用 `:` 指明类别。
- 文本文件。一个域名一行。支持多种匹配模式:
  - 以 `domain:` 开头或省略，子域名匹配。
  - 以 `keyword:` 开头，关键字匹配。
  - 以 `regexp:` 开头，正则匹配(Golang 标准)。
  - 以 `full:` 开头，完整匹配。

IP 表可以是:

- v2ray `geoip.dat` 文件。需用 `:` 指明类别。
- 文本文件。每行一个 IP 或 CIDR，支持 IPv6。

## 示例

```shell
mosdns-cn -s :53 --local-upstream https://223.5.5.5/dns-query --local-domain geosite.dat:cn --local-ip geoip.dat:cn --remote-upstream https://8.8.8.8/dns-query --remote-domain geosite.dat:geolocation-!cn
```

## 相关连接

- [mosdns](https://github.com/IrineSistiana/mosdns)
- [V2Ray 路由规则文件加强版](https://github.com/Loyalsoldier/v2ray-rules-dat)

## Open Source Components / Libraries / Reference

依赖

* [IrineSistiana/mosdns](https://github.com/IrineSistiana/mosdns): [GPL-3.0 License](https://github.com/IrineSistiana/mosdns/blob/main/LICENSE)
* [uber-go/zap](https://github.com/uber-go/zap): [LICENSE](https://github.com/uber-go/zap/blob/master/LICENSE.txt)
* [miekg/dns](https://github.com/miekg/dns): [LICENSE](https://github.com/miekg/dns/blob/master/LICENSE)
* [jessevdk/go-flags](https://github.com/jessevdk/go-flags): [BSD-3-Clause License](https://github.com/jessevdk/go-flags/blob/master/LICENSE)

---

Made with love ♥