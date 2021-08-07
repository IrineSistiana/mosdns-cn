# mosdns-cn

只是一个 DNS 转发器。

- 上游服务器支持 UDP/TCP/DoT/DoH 协议。支持 socks5 代理。支持连接复用，低响应延时。
- 可选本地/远程 DNS 分流。可以同时根据域名和 IP 分流，更准确。
- 单一可执行文件，没有依赖，无需配置文件，三分钟一行命令开箱即用。

## 参数

```text
  # 基本参数
  -s, --server:           (必需) 监听地址。会同时监听 UDP 和 TCP。
  -c, --cache:            (可选) 缓存大小。单位: 条。默认无缓存。
      --min-ttl:          (可选) 应答的最小 TTL。单位: 秒。
      --max-ttl:          (可选) 应答的最大 TTL。单位: 秒。
      --hosts:            (可选) Hosts 表。这个参数可出现多次，会从多个表载入数据。
      --arbitrary:        (可选) Arbitrary 表。这个参数可出现多次，会从多个表载入数据。
      --blacklist-domain: (可选) 黑名单域名表。这些域名会被 NXDOMAIN 屏蔽。这个参数可出现多次，会从多个表载入数据。

  # 上游
  # 如果无需分流，只需配置这个参数:
      --upstream:         (必需) 上游服务器。这个参数可出现多次来配置多个上游。会并发请求所有上游。
  # 如果需要分流，配置以下参数:
      --local-upstream:   (必需) 本地上游服务器。这个参数可出现多次来配置多个上游。会并发请求所有上游。
      --local-ip:         (必需) 本地 IP 地址表。这个参数可出现多次，会从多个表载入数据。
      --local-domain:     (可选) 本地域名表。这些域名会被本地上游解析。这个参数可出现多次，会从多个表载入数据。
      --local-latency:    (可选) 本地上游服务器延时，单位毫秒。指示性参数，防止本地上游被远程上游抢答。
      --remote-upstream:  (必需) 远程上游服务器。这个参数可出现多次来配置多个上游。会并发请求所有上游。
      --remote-domain:    (可选) 远程域名表。这些域名会被远程上游解析。这个参数可出现多次，会从多个表载入数据。

  # 其他
  -v, --debug             更详细的调试 log。你可以看到每个域名的分流的过程。
      --log-file:         将日志写入文件。
      --dir:              工作目录。
      --cd2exe            自动将可执行文件的目录作为工作目录。
      --service:[install|uninstall|start|stop|restart] 控制系统服务。
      --insecure          跳过 TLS 服务器身份验证。谨慎使用。
      --ca:               指定验证服务器身份的 CA 证书。PEM 格式，可以是证书包(bundle)。这个参数可出现多次来载入多个文件。
```

## 使用示例

仅转发，不分流:

```shell
mosdns-cn -s :53 --upstream https://8.8.8.8/dns-query
```

根据 [V2Ray 路由规则文件加强版](https://github.com/Loyalsoldier/v2ray-rules-dat) 的 `geosite.dat` 域名和 `geoip.dat` IP 资源分流本地/远程域名并且屏蔽广告域名:

```shell
mosdns-cn -s :53 --blacklist-domain 'geosite:category-ads-all' --local-upstream https://223.5.5.5/dns-query --local-domain geosite.dat:cn --local-ip geoip.dat:cn --remote-upstream https://8.8.8.8/dns-query --remote-domain 'geosite.dat:geolocation-!cn'
```

### 使用 `--service` 一键将 mosdns-cn 安装到系统服务实现自启

- 可用于 `Windows XP+, Linux/(systemd | Upstart | SysV), and OSX/Launchd` 平台。
- 安装成功后程序将跟随系统自启。
- 需要管理员或 root 权限运行 mosdns-cn。
- `--service install` 无 `--dir` 参数时会默认使用程序所在的目录作为工作目录。

示例:

```shell
# 安装 
# mosdns-cn --service install [+其他参数...]
mosdns-cn --service install -s :53 --upstream https://8.8.8.8/dns-query

# 安装成功后需手动启动服务才能使用。因为服务虽然会跟随系统自启，但安装成功后并不会。
mosdns-cn --service start

# 卸载
mosdns-cn --service stop
mosdns-cn --service uninstall
```

## 详细参数说明

### 上游 upstream

上游支持 4 种协议。示例:

- UDP: `8.8.8.8`, `208.67.222.222:443`
- TCP: `tcp://8.8.8.8`
- DoT: `tls://8.8.8.8`, `tls://dns.google`
- DoH: `https://8.8.8.8/dns-query`, `https://dns.google/dns-query`

支持 3 个格外参数:

- `netaddr` 可以手动指定服务器域名的实际地址(`ip:port`)。e.g. `tls://dns.google?netaddr=8.8.8.8:853`
- `socks5` 通过 socks5 代理服务器连接上游。暂不支持 UDP 协议。e.g. `tls://dns.google?socks5=127.0.0.1:1080`
- `keepalive` 连接复用空连接保持时间。单位: 秒。不同协议默认值不同。DoH: 30 (默认启用连接复用)，TCP/DoT: 0 (默认禁用连接复用)。
  - 启用连接复用后，只有第一个请求需要建立连接和握手，接下来的请求会在同一连接中直接传送。所以平均请求延时会和 UDP 一样低。
  - 不是什么黑科技，是 RFC 标准。绝大多数知名的公共 DNS 提供商都支持连接复用。比如 Cloudflare，Google，AliDNS。
  - e.g. `tls://dns.google?keepalive=10`
- 如需同时使用多个参数，在地址后加 `?` 然后参数之间用 `&` 分隔
  - e.g. `tls://dns.google?netaddr=8.8.8.8:853&keepalive=10&socks5=127.0.0.1:1080`

### 域名表

- 可以是 v2ray `geosite.dat` 文件。需用 `:` 指明类别。
- 可以是文本文件。一个域名一行。匹配规则:
  - 以 `domain:` 开头或省略，子域名匹配。
  - 以 `keyword:` 开头，关键字匹配。
  - 以 `regexp:` 开头，正则匹配(Golang 标准)。
  - 以 `full:` 开头，完整匹配。

### IP 表

- 可以是 v2ray `geoip.dat` 文件。需用 `:` 指明类别。
- 可以是文本文件。每行一个 IP 或 CIDR。支持 IPv6。

### Hosts 表

格式:

域名在前，IP 在后。支持一行多个 IP，支持 IPv6。支持多行合并。

- 以 `full:` 开头或省略，完整匹配。
- 以 `domain:` 开头，域匹配。
- 以 `keyword:` 开头，关键字匹配。
- 以 `regexp:` 开头，正则匹配(Golang 标准)。

示例:

```txt
# 支持一行多个 IP
dns.google 8.8.8.8 2001:4860:4860::8888 
# 支持多行合并。下面的 IP 会和上面的数据合并在一起，而不是覆盖。
dns.google 8.8.4.4   

# 其他的匹配方式
domain:dns.cloudflare.com 104.16.133.229
keyword:dns.google 8.8.8.8
regexp:^dns\.google$ 8.8.8.8
```

### Arbitrary 表

Arbitrary 可以构建任意应答。

格式示例:

```txt
# [qName]   [qClass]  [qType] [section] [RFC 1035 resource record]
dns.google  IN        A       ANSWER    dns.google. IN A 8.8.8.8
dns.google  IN        A       ANSWER    dns.google. IN A 8.8.4.4
dns.google  IN        AAAA    ANSWER    dns.google. IN AAAA 2001:4860:4860::8888
example.com IN        A       NA        example.com.  IN  SOA   ns.example.com. username.example.com. ( 2020091025 7200 3600 1209600 3600 )
```

- `qName`: 请求的域名。默认是完整匹配。其他匹配规则:
  - 以 `full:` 开头或省略，完整匹配。
  - 以 `domain:` 开头，子域名匹配。
  - 以 `keyword:` 开头，关键字匹配。
  - 以 `regexp:` 开头，正则匹配(Golang 标准)。
- `qClass`, `qType`: 请求的类型。可以是字符，必须大写，支持绝大数的类型。如不支持，也可以是数字。
- `section`: 该资源记录在应答的位置。可以是 `ANSWER`, `NS`, `EXTRA`。
- `RFC 1035 resource record`: RFC 1035 格式的资源记录 (resource record)
  。不支持换行，域名不支持缩写。具体格式可以参考 [Zone file](https://en.wikipedia.org/wiki/Zone_file) 或自行搜索。

如果 `qName`,  `qClass`, `qType` 成功匹配请求，则将对应的 `RFC 1035 resource record` 的记录放在应答 `section` 部分。然后返回应答。

## 运行顺序

各个功能运行顺序:

1. 查找 hosts
2. 查找 arbitrary
3. 查找 blacklist-domain 域名黑名单
4. 查找 cache 缓存
5. 转发至上游

分流模式中上游的转发顺序:

1. 非 A/AAAA 类型的请求将直接使用 `--local-upstream` 本地上游。
2. 如果请求的域名匹配到 `--local-domain` 本地域名。则直接使用 `--local-upstream` 本地上游。
3. 如果请求的域名匹配到 `--remote-domain` 远程域名。则直接使用`--remote-upstream` 远程上游。
4. 否则，先转发至本地上游获取应答。
   - 如果本地上游的应答包含 `--local-ip` 本地 IP。则直接采用本地上游的结果
   - 否则使用远程上游。

## 相关连接

- [mosdns](https://github.com/IrineSistiana/mosdns): 插件化 DNS 路由/转发器。
- [V2Ray 路由规则文件加强版](https://github.com/Loyalsoldier/v2ray-rules-dat): 常用域名/IP 资源一步到位。

## Open Source Components / Libraries / Reference

依赖

- [IrineSistiana/mosdns](https://github.com/IrineSistiana/mosdns): [GPL-3.0 License](https://github.com/IrineSistiana/mosdns/blob/main/LICENSE)
- [uber-go/zap](https://github.com/uber-go/zap): [LICENSE](https://github.com/uber-go/zap/blob/master/LICENSE.txt)
- [miekg/dns](https://github.com/miekg/dns): [LICENSE](https://github.com/miekg/dns/blob/master/LICENSE)
- [jessevdk/go-flags](https://github.com/jessevdk/go-flags): [BSD-3-Clause License](https://github.com/jessevdk/go-flags/blob/master/LICENSE)
- [kardianos/service](https://github.com/kardianos/service): [zlib](https://github.com/kardianos/service/blob/master/LICENSE)
