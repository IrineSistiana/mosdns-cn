# mosdns-cn

一个 DNS 转发器。

- 上游支持 UDP/TCP/DoT/DoH 协议。
- TCP/DoT/DoH 协议均支持连接复用，0 RTT，延时和 UDP 一样低。
- 支持域名屏蔽(广告屏蔽)，修改 ttl，hosts 等常用功能。
- 支持 lazy cache 机制，可以优化糟糕网络环境下的应答响应时间。
- 支持 Redis 外部缓存，可以实现重启程序不会丢缓存。
- 可选本地/远程 DNS 分流。可以同时根据域名和 IP 分流，更准确。
  - 支持从文本文件载入数据。通用格式(IP 就是 IP，域名就是域名，一条一行)。
  - 支持直接从 v2ray 的 `geoip.dat` 和 `geosite.dat` 载入数据。
  - 支持从多个文件载入数据。
- 无需折腾。三分钟完成配置。常见平台支持命令行一键安装。

## 参数和命令

```text
  -s, --server:           (必需) 监听地址。会同时监听 UDP 和 TCP。
  
  -c, --cache:            内置内存缓存大小。单位: 条。默认无缓存。
      --redis-cache:      Redis 外部缓存地址。
                          TCP 连接: `redis://<user>:<password>@<host>:<port>/<db_number>`
                          Unix 连接: `unix://<user>:<password>@</path/to/redis.sock>?db=<db_number>`
      --lazy-cache-ttl:   Lazy cache 生存时间。单位: 秒。如果设定，应答会在缓存中生
                          存 lazy_cache_ttl 秒。如果命中过期的应答，则会立即返回 TTL 为
                          lazy_cache_reply_ttl 的应答，然后后台去更新该应答。
      --lazy-cache-reply-ttl: 返回的过期缓存的 TTL 会被设定成该值。默认 30 (RFC 8767 的建议值)。
                            
      --min-ttl:          应答的最小 TTL。单位: 秒。
      --max-ttl:          应答的最大 TTL。单位: 秒。
 
      --hosts:            Hosts 表。这个参数可出现多次，会从多个表载入数据。
      --arbitrary:        Arbitrary 表。这个参数可出现多次，会从多个表载入数据。
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
      
   # 小工具命令(非设置性参数)
      --service:[install|uninstall|start|stop|restart] 控制系统服务。
      --gen-config:       生成一个 yaml 配置文件模板到指定位置。
      --version           打印程序版本。
```

yaml 配置文件中可设定以下参数:

```yaml
server_addr: ""
cache_size: 0
lazy_cache_ttl: 0
lazy_cache_reply_ttl: 0
redis_cache: ""
min_ttl: 0
max_ttl: 0
hosts: []
arbitrary: []
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
资源分流本地/远程域名并且屏蔽广告域名:

```shell
mosdns-cn -s :53 --blacklist-domain "geosite.dat:category-ads-all" --local-upstream https://223.5.5.5/dns-query --local-domain "geosite.dat:cn" --local-ip "geoip.dat:cn" --remote-upstream https://8.8.8.8/dns-query --remote-domain "geosite.dat:geolocation-!cn"
```

使用配置文件:

好处: 直观，更改设置更方便，无需修改启动参数。

```shell
# 生成一个配置文件模板到当前目录。
mosdns-cn --gen-config ./my-config.yaml
# 载入配置文件。
mosdns-cn --config ./my-config.yaml
```

### 使用 `--service` 一键将 mosdns-cn 安装到系统服务实现自启

- 可用于 `Windows XP+, Linux/(systemd | Upstart | SysV), and OSX/Launchd` 平台。
- 安装成功后程序将跟随系统自启。
- 需要管理员或 root 权限运行 mosdns-cn。
- 某些平台使用相对路径会导致服务找不到 yaml 配置文件和其他资源文件。如果遇到通过命令行运行可以正常启动但安装成服务后不能启动的玄学问题，可以尝试把所有路径换成绝对路径后重新安装 mosdns-cn。

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

支持四种协议。省略协议默认为 UDP 协议。省略端口号会使用协议默认值。格式示例:

- UDP: `8.8.8.8`, `208.67.222.222:443`。
- TCP: `tcp://8.8.8.8`。
- DoT: IP 直连 `tls://8.8.8.8` ，域名 `tls://dns.google`。
- DoH: IP 直连 `https://8.8.8.8/dns-query` ，域名 `https://dns.google/dns-query` 。

Tips: 如果服务器支持的话，优先使用 IP 直连。解析服务器地址会有格外消耗。

支持以下格外参数:

- `netaddr`: 有些服务器只能使用域名地址(TLS 必须有 SNI)，该参数可手动为域名地址指定 IP 和端口。省略端口号会使用协议默认值。
  - **当本机运行 mosdns 并且将系统 DNS 指向 mosdns 时，必须为域名地址指定 IP 地址，否则会出现解析死循环。**
  - e.g. `tls://dns.google?netaddr=8.8.8.8:853`
- `socks5`: 通过 socks5 代理服务器连接上游。暂不支持 socks5 UDP 协议和用户名密码认证。e.g. `tls://dns.google?socks5=127.0.0.1:1080`
- `keepalive`: TCP/DoT/DoH 连接复用空连接保持时间。单位: 秒。默认: TCP/DoT: 10。DoH: 30。
  - e.g. `tls://8.8.8.8?keepalive=10`
- 如需同时使用多个参数，在地址后加 `?` 然后参数之间用 `&` 分隔
  - e.g. `tls://dns.google?netaddr=8.8.8.8:853&keepalive=10&socks5=127.0.0.1:1080`

高级功能，可以启用最新的功能，但并非所有服务器都适用:

- 参数 `enable_http3=true`: 将使用 HTTP/3 (aka. HTTP over QUIC) 连接 DoH 服务器。需要服务器支持。
  - Google 搜 `http3 test`，有在线 HTTP3 测试的网站可以测试 DoH 服务器是否支持 HTTP/3。
- 参数 `enable_pipeline=true`: TCP/DoT 新的 pipeline 连接复用模式。性能更好延时更低效率更高。要服务器支持。
  - [mosdns](https://github.com/IrineSistiana/mosdns) 有一个命令可以探测服务器是否支持 pipeline。
- 特殊协议 UDPME: 仍然是 UDP 协议，但要求服务器支持 EDNS0。可以借助 EDNS0 的机制过滤掉某些有问题的(假的) UDP 报文。实验性功能，使用条件特殊，不保证稳定性和可用性。
  - 运行命令 `dig +edns cloudflare.com @要测试的服务器`，观察返回的应答中是否包含 `EDNS: version: 0`。如果有则服务器支持 EDNS0。
  - e.g. `udpme://8.8.8.8`。

### 域名表

- 可以是 v2ray `geosite.dat` 文件。需用 `:` 指明类别。
- 可以是文本文件。一个域名规则一行。如果域名匹配方式被省略，则默认是 `domain` 匹配。域名匹配方式详见 [这里](#域名匹配规则)。

### IP 表

- 可以是 v2ray `geoip.dat` 文件。需用 `:` 指明类别。
- 可以是文本文件。每行一个 IP 或 CIDR。支持 IPv6。

### Hosts 表

注: 虽然都叫 hosts，但 mosdns-cn 所用的格式和平常 Win，Linux 系统内的那个 hosts 文件不一样。

格式:

- 域名规则在前，IP 在后。支持一行多个 IP，支持 IPv6。支持多行合并。
- 如果域名匹配规则的方式被省略，则默认是 `full` 完整匹配。域名匹配规则详见 [这里](#域名匹配规则)。

格式示例:

```txt
# [域名匹配规则] [IP...]
dns.google 8.8.8.8 2001:4860:4860::8888 ...
# 支持多行合并，会和上面的数据合并在一起，而不是覆盖。
dns.google 8.8.4.4
```

### Arbitrary 表

Arbitrary 可以构建任意应答。

格式示例:

```txt
# [域名匹配规则]  [qClass]  [qType] [section] [RFC 1035 resource record ...]
dns.google      IN        A       ANSWER    dns.google. IN A 8.8.8.8
dns.google      IN        A       ANSWER    dns.google. IN A 8.8.4.4
dns.google      IN        AAAA    ANSWER    dns.google. IN AAAA 2001:4860:4860::8888
example.com     IN        A       NA        example.com.  IN  SOA   ns.example.com. username.example.com. ( 2020091025 7200 3600 1209600 3600 )
```

- `域名匹配规则`: 如果匹配方式被省略，则默认是 `full` 完整匹配。 域名匹配方式详见 [域名匹配规则](#域名匹配规则)。
- `qClass`, `qType`: 请求的类型。可以是字符，比如 `A`，`AAAA` 等，必须大写，理论上支持所有类型。如果遇到不支持，也可以换成对应数字。
- `section`: 该资源记录在应答的位置。可以是 `ANSWER`, `NS`, `EXTRA`。
- `RFC 1035 resource record`: RFC 1035 格式的资源记录 (resource record)。不支持换行，域名不支持缩写。具体格式可以参考 [Zone file](https://en.wikipedia.org/wiki/Zone_file) 或自行搜索。

如果 `域名匹配规则`,  `qClass`, `qType` 成功匹配请求，则将所有对应的 `RFC 1035 resource record` 的记录放在应答 `section` 部分。然后返回应答。

## 运行顺序

各个功能运行顺序:

1. 查找 hosts
2. 查找 arbitrary
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

域名规则有多个匹配方式 (和 v2ray 一致):

- 以 `domain:` 开头，域匹配。e.g: `domain:google.com` 会匹配自身 `google.com`，以及其子域名 `www.google.com`, `maps.l.google.com` 等。
- 以 `full:` 开头，完整匹配。e.g: `full:google.com` 只会匹配自身。
- 以 `keyword:` 开头，关键字匹配。e.g: `keyword:google.com` 会匹配包含这个字段的域名，如 `google.com.hk`, `www.google.com.hk`。
- 以 `regexp:` 开头，正则匹配([Golang 标准](https://github.com/google/re2/wiki/Syntax))。e.g: `regexp:.+\.google\.com$`。

匹配顺序(也和 v2ray 一致): `full` > `domain` > `regexp` > `keyword`。

## Open Source Components / Libraries / Reference

依赖

- [IrineSistiana/mosdns](https://github.com/IrineSistiana/mosdns): [GPL-3.0 License](https://github.com/IrineSistiana/mosdns/blob/main/LICENSE)
- [uber-go/zap](https://github.com/uber-go/zap): [LICENSE](https://github.com/uber-go/zap/blob/master/LICENSE.txt)
- [miekg/dns](https://github.com/miekg/dns): [LICENSE](https://github.com/miekg/dns/blob/master/LICENSE)
- [jessevdk/go-flags](https://github.com/jessevdk/go-flags): [BSD-3-Clause License](https://github.com/jessevdk/go-flags/blob/master/LICENSE)
- [kardianos/service](https://github.com/kardianos/service): [zlib](https://github.com/kardianos/service/blob/master/LICENSE)
