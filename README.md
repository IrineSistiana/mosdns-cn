# mosdns-cn

一个 DNS 转发器。

- 支持主流加密 DNS 协议。
- 支持标准化的连接复用技术，效率更高延时更低。
- 支持请求屏蔽，修改 ttl，hosts 等常用功能。
- 可选本地/远程 DNS 分流。可以同时根据域名和 IP 分流，更准确。并发请求，无格外延时。
- 无需折腾。三分钟完成配置。常见平台支持命令行一键安装。

## 参数和命令

```text
  -s, --server:           (必需) 监听地址。会同时监听 UDP 和 TCP。
  
  -c, --cache:            内置内存缓存大小。单位: 条。
      --redis-cache:      Redis 外部缓存地址。
                          TCP 连接: `redis://<user>:<password>@<host>:<port>/<db_number>`
                          Unix 连接: `unix://<user>:<password>@</path/to/redis.sock>?db=<db_number>`
      --lazy-cache-ttl:   Lazy cache 生存时间。单位: 秒。
      --lazy-cache-reply-ttl: 返回的过期缓存的 TTL 会被设定成该值。默认 30 (RFC 8767 的建议值)。
                            
      --min-ttl:          应答的最小 TTL。单位: 秒。
      --max-ttl:          应答的最大 TTL。单位: 秒。
 
      --hosts:            Hosts 表。这个参数可出现多次，会从多个表载入数据。
      --blacklist-domain: 黑名单域名表。这些域名会被 NXDOMAIN 屏蔽。这个参数可出现多次，会从多个表载入数据。
      --ca:               指定验证服务器身份的 CA 证书。PEM 格式，可以是证书包(bundle)。这个参数可出现多次来载入多个文件。
      --insecure          跳过 TLS 服务器身份验证。谨慎使用。
  -v, --debug             更详细的调试 log。可以看到每个域名的分流的过程。
      --log-file:         将日志写入文件。

  # 上游
  # 如果无需分流，只需配置下面这个参数:
      --upstream:         (必需) 上游服务器。这个参数可出现多次来配置多个上游。会并发请求所有上游。
  # 如果需要分流，配置以下参数:
      --local-upstream:   (必需) 本地上游服务器。这个参数可出现多次来配置多个上游。会并发请求所有上游。
      --local-ip:         (必需) 本地 IP 地址表。这个参数可出现多次，会从多个表载入数据。
      --local-domain:     本地域名表。这些域名会被本地上游解析。这个参数可出现多次，会从多个表载入数据。
      --local-latency:    本地上游服务器延时，单位毫秒。默认: 50。指示性参数，保护本地上游不被远程上游抢答。
      --remote-upstream:  (必需) 远程上游服务器。这个参数可出现多次来配置多个上游。会并发请求所有上游。
      --remote-domain:    远程域名表。这些域名会被远程上游解析。这个参数可出现多次，会从多个表载入数据。

   # 其他
      --config:           从 yaml 配置文件载入参数。
      --dir:              工作目录。
      --cd2exe            自动将可执行文件的目录作为工作目录。
      
   # 小工具命令
      --service [install|uninstall|start|stop|restart] 控制系统服务。
      --gen-config:       生成一个 yaml 配置文件模板到指定位置。
      --version           打印程序版本。
```

yaml 配置支持以下参数:

```yaml
server_addr: ""
cache_size: 0
lazy_cache_ttl: 0
lazy_cache_reply_ttl: 0
redis_cache: ""
min_ttl: 0
max_ttl: 0
hosts: []
blacklist_domain: []
insecure: false
ca: []
debug: false
log_file: ""
upstream: []
local_upstream: []
local_ip: []
local_domain: []
local_latency: 50
remote_upstream: []
remote_domain: []
working_dir: ""
cd2exe: false
```

## 使用示例

仅转发，不分流:

```shell
mosdns-cn -s :53 --upstream https://8.8.8.8/dns-query
```

根据 [V2Ray 路由规则文件加强版](https://github.com/Loyalsoldier/v2ray-rules-dat) 的 `geosite.dat` 域名和 `geoip.dat` IP
资源分流本地/远程域名并且屏蔽广告域名。下载这两个文件放在 mosdns-cn 的目录以下命令就可以直接使用了。

```shell
mosdns-cn -s :53 --blacklist-domain "geosite.dat:category-ads-all" --local-upstream https://1.12.12.12/dns-query --local-domain "geosite.dat:cn" --local-ip "geoip.dat:cn" --remote-upstream https://8.8.8.8/dns-query --remote-domain "geosite.dat:geolocation-!cn"
```

使用配置文件:

```shell
# 生成一个配置文件模板到当前目录。
mosdns-cn --gen-config ./my-config.yaml
# 编辑配置。
# 载入配置文件。
mosdns-cn --config ./my-config.yaml
```

### 使用 `--service` 将 mosdns-cn 注册到系统服务实现开机自启

- 理论上可用于 `Windows XP+, Linux/(systemd | Upstart | SysV), and OSX/Launchd` 平台。
- OSX 平台需要在系统设置的安全性与隐私里赋予 mosdns-cn 完全磁盘访问权限。否则 mosdns-cn 无法读取配置和资源文件。

示例:

```shell
# 安装成系统服务(注册启动项) 
# mosdns-cn --service install [+其他参数...]
mosdns-cn --service install -s :53 --upstream https://8.8.8.8/dns-query

# 安装成功后需手动启动服务才能使用。因为服务只会跟随系统自启，安装成功后并不会。
mosdns-cn --service start

# 卸载
mosdns-cn --service stop
mosdns-cn --service uninstall
```

## 详细参数说明

### 上游 upstream

省略协议默认为 UDP 协议。省略端口号会使用协议默认值。

- UDP: `8.8.8.8`, `208.67.222.222:443`。
- TCP: `tcp://8.8.8.8`。
- DoT: IP 直连 `tls://8.8.8.8` ，域名 `tls://dns.google`。
- DoH: IP 直连 `https://8.8.8.8/dns-query` ，域名 `https://dns.google/dns-query` 。
- UDPME: `udpme://8.8.8.8`。
  - 这是个能过滤掉 UDP 抢答应答的方案。仍然是 UDP 协议。服务器必须支持 EDNS0。如果抢答者不支持 EDNS0，则可以 100% 过滤抢答应答。
  - Tips: `dig +edns cloudflare.com @服务器地址` 观察返回是否有一行 `EDNS: version: 0` 来确定服务器是否支持 EDNS0。

地址 URL 中还可以配置以下参数:

- `netaddr`: 为域名地址指定 IP。支持端口号。
  - 如果本机自用的 mosdns-cn (本机系统的 DNS 服务器是为本机的 mosdns-cn)，**必须**为域名地址设定 IP，否则会出现解析死循环。
  - e.g. `tls://dns.google?netaddr=8.8.8.8:853`
- `socks5`: 通过 socks5 代理服务器连接上游。暂不支持 UDP 和 HTTP3 以及用户名密码认证。
  - e.g. `tls://8.8.8.8?socks5=127.0.0.1:1080`
- `enable_http3=true`: DoH 使用 HTTP/3 连接服务器。必须确定服务器支持后再启用该选项。
  - 已知 Google 和 Cloudflare 的 DoH 是支持 HTTP/3 的。
  - e.g. `https://8.8.8.8/dns-query?enable_http3=true`
- `enable_pipeline=true`: TCP/DoT 使用 RFC 7766 新的 query pipelining 连接复用模式而不是 RFC 1035 的连接重用模式。所需连接更少，效率更高。必须确定服务器支持后再启用该选项，否则会出问题。
  - 已知 DNSPod，Google 和 Cloudflare 的 TCP/DoT 是支持该模式的。大多数知名公用 DNS 服务器都支持该模式。
  - [mosdns](https://github.com/IrineSistiana/mosdns) 有一个命令可以探测服务器是否支持 pipeline。
  - e.g. `tls://8.8.8.8?enable_pipeline=true`
- `keepalive`: TCP/DoT/DoH 连接复用最长空连接保持时间。单位: 秒。默认: TCP/DoT: 10。DoH: 30。一般不需要改。
  - e.g. `tls://8.8.8.8?keepalive=10`
- 如需同时设置多个参数，在地址后加 `?` 然后参数之间用 `&` 分隔
  - e.g. `tls://dns.google?netaddr=8.8.8.8:853&keepalive=10&socks5=127.0.0.1:1080`

### 域名表

- 可以是 v2ray `geosite.dat` 文件。需用 `:` 指明类别。
- 可以是文本文件。一个域名规则一行。如果域名匹配方式被省略，则默认是 `domain` 匹配。域名匹配方式详见 [这里](#域名匹配规则)。

### IP 表

- 可以是 v2ray `geoip.dat` 文件。需用 `:` 指明类别。
- 可以是文本文件。每行一个 IP 或 CIDR。支持 IPv6。

### Hosts 表

注: 虽然都叫 hosts，但 mosdns-cn 所用的格式和平常 Win，Linux 系统内的那个 hosts 文件不一样。

格式:

- 域名规则在前，IP 在后，空格分割。支持一行多个 IP，支持 IPv6。
- 如果域名匹配规则的方式被省略，则默认是 `full` 完整匹配。域名匹配规则详见 [这里](#域名匹配规则)。

格式示例:

```txt
# [域名匹配规则] [IP...]
dns.google 8.8.8.8 2001:4860:4860::8888 ...
```

## 运行顺序

各个功能运行顺序:

1. 查找 hosts
3. 查找 blacklist-domain 域名黑名单
4. 查找 cache 缓存
5. 转发至上游

分流模式中上游的转发顺序:

1. 非 A/AAAA 类型的请求将直接使用 `--local-upstream` 本地上游。结束。
2. 如果请求的域名匹配到 `--local-domain` 本地域名。则直接使用 `--local-upstream` 本地上游。结束。
3. 如果请求的域名匹配到 `--remote-domain` 远程域名。则直接使用`--remote-upstream` 远程上游。结束。
4. 同时转发至本地上游获取应答。
5. 如果本地上游的应答包含 `--local-ip` 本地 IP。则直接采用本地上游的结果。结束。
6. 否则采用远程上游的结果。结束。

## 域名匹配规则

域名规则有多个匹配方式 (和 [v2fly/domain-list-community](https://github.com/v2fly/domain-list-community) 一致):

- 以 `domain:` 开头，域匹配。e.g: `domain:google.com` 会匹配自身 `google.com`，以及其子域名 `www.google.com`, `maps.l.google.com` 等。
- 以 `full:` 开头，完整匹配。e.g: `full:google.com` 只会匹配自身。
- 以 `keyword:` 开头，关键字匹配。e.g: `keyword:google.com` 会匹配包含这个字段的域名，如 `google.com.hk`, `www.google.com.hk`。
- 以 `regexp:` 开头，正则匹配([Golang 标准](https://github.com/google/re2/wiki/Syntax))。e.g: `regexp:.+\.google\.com$`。

匹配优先级(和 v2ray 优先级逻辑一致): `full` > `domain` > `regexp` > `keyword`。

## 更多功能

如果需要更多功能，如更多分流策略等，请使用 [mosdns](https://github.com/IrineSistiana/mosdns)。

## Open Source Components / Libraries / Reference

依赖

- [IrineSistiana/mosdns](https://github.com/IrineSistiana/mosdns): [GPL-3.0 License](https://github.com/IrineSistiana/mosdns/blob/main/LICENSE)
- [uber-go/zap](https://github.com/uber-go/zap): [LICENSE](https://github.com/uber-go/zap/blob/master/LICENSE.txt)
- [miekg/dns](https://github.com/miekg/dns): [LICENSE](https://github.com/miekg/dns/blob/master/LICENSE)
- [jessevdk/go-flags](https://github.com/jessevdk/go-flags): [BSD-3-Clause License](https://github.com/jessevdk/go-flags/blob/master/LICENSE)
- [kardianos/service](https://github.com/kardianos/service): [zlib](https://github.com/kardianos/service/blob/master/LICENSE)
