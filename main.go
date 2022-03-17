//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/load_cache"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/elem"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/msg_matcher"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/netlist"
	_ "github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/server"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/plugin/executable/arbitrary"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/plugin/executable/cache"
	fastforward "github.com/IrineSistiana/mosdns/v3/dispatcher/plugin/executable/fast_forward"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/plugin/executable/hosts"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/plugin/executable/ttl"
	"github.com/jessevdk/go-flags"
	"github.com/kardianos/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
)

var version = "dev/unknown"

type Opt struct {
	ConfigFile        string   `long:"config" description:"Load settings from the yaml file" yaml:"-"`
	ServerAddr        string   `short:"s" long:"server" description:"Server address" yaml:"server_addr"`
	CacheSize         int      `short:"c" long:"cache" description:"Cache size"  yaml:"cache_size"`
	LazyCacheTTL      int      `long:"lazy-cache-ttl" description:"Responses will stay in the cache for configured seconds." yaml:"lazy_cache_ttl"`
	LazyCacheReplyTTL int      `long:"lazy-cache-reply-ttl" description:"TTL value to use when replying with expired data." yaml:"lazy_cache_reply_ttl"`
	RedisCache        string   `long:"redis-cache" description:"Redis cache backend." yaml:"redis_cache"`
	MinTTL            uint32   `long:"min-ttl" description:"Minimum TTL value for DNS responses" yaml:"min_ttl"`
	MaxTTL            uint32   `long:"max-ttl" description:"Maximum TTL value for DNS responses" yaml:"max_ttl"`
	Hosts             []string `long:"hosts" description:"Hosts" yaml:"hosts"`
	Arbitrary         []string `long:"arbitrary" description:"Arbitrary record" yaml:"arbitrary"`
	BlacklistDomain   []string `long:"blacklist-domain" description:"Blacklist domain" yaml:"blacklist_domain"`
	Insecure          bool     `long:"insecure" description:"Disable TLS certificate validation" yaml:"insecure"`
	CA                []string `long:"ca" description:"CA files" yaml:"ca"`
	Debug             bool     `short:"v" long:"debug" description:"Verbose log" yaml:"debug"`
	LogFile           string   `long:"log-file" description:"Write logs to a file" yaml:"log_file"`

	// simple forwarder
	Upstream []string `long:"upstream" description:"Upstream" yaml:"upstream"`

	// local/remote forwarder
	LocalUpstream  []string `long:"local-upstream" description:"Local upstream" yaml:"local_upstream"` // required if Upstream is empty
	LocalIP        []string `long:"local-ip" description:"Local ip" yaml:"local_ip"`
	LocalDomain    []string `long:"local-domain" description:"Local domain" yaml:"local_domain"`
	LocalLatency   int      `long:"local-latency" description:"Local latency in milliseconds" default:"50" yaml:"local_latency"`
	RemoteUpstream []string `long:"remote-upstream" description:"Remote upstream" yaml:"remote_upstream"` // required if Upstream is empty
	RemoteDomain   []string `long:"remote-domain" description:"Remote domain" yaml:"remote_domain"`

	WorkingDir   string `long:"dir" description:"Working dir" yaml:"working_dir"`
	CD2Exe       bool   `long:"cd2exe" description:"Change working dir to executable automatically" yaml:"cd2exe"`
	Service      string `long:"service" description:"Service control" choice:"install" choice:"uninstall" choice:"start" choice:"stop" choice:"restart" yaml:"-"`
	RunAsService bool   `short:"S" description:"Run as a system service" hidden:"true" yaml:"-"`

	GenConfig    string `long:"gen-config" description:"Generate a configuration file to the given path" yaml:"-"`
	PrintVersion bool   `long:"version" description:"Print the program version" yaml:"-"`
}

var opt = new(Opt)

func main() {
	_, err := flags.Parse(opt)
	if err != nil { // error msg has been printed by flags
		os.Exit(1)
	}

	if opt.PrintVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	cd() // change wd for cmd line arguments

	if p := opt.GenConfig; len(p) > 0 {
		f, err := os.Create(p)
		if err != nil {
			mlog.S().Fatal(err)
		}
		defer f.Close()

		encoder := yaml.NewEncoder(f)
		encoder.SetIndent(2)
		defer encoder.Close()
		err = encoder.Encode(opt)
		if err != nil {
			mlog.S().Fatal(err)
		}
		os.Exit(0)
	}

	if cf := opt.ConfigFile; len(cf) > 0 {
		b, err := os.ReadFile(cf)
		if err != nil {
			mlog.S().Fatalf("failed to load configuration file: %v", err)
		}
		if err := yaml.Unmarshal(b, opt); err != nil {
			mlog.S().Fatalf("failed to parse configuration file: %v", err)
		}
	}
	cd() // change wd for config arguments

	if opt.Debug {
		mlog.Level().SetLevel(zap.DebugLevel)
	} else {
		mlog.Level().SetLevel(zap.InfoLevel)
	}

	if len(opt.Service) == 0 && !opt.RunAsService {
		go run()
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM)
		s := <-c
		mlog.S().Infof("%s, exiting", s)
		os.Exit(0)
	}

	svcConfig := &service.Config{
		Name:        "mosdns-cn",
		DisplayName: "mosdns-cn",
		Description: "A DNS forwarder",
	}

	svc := new(svc)
	s, err := service.New(svc, svcConfig)
	if err != nil {
		mlog.S().Fatalf("failed to init service: %v", err)
	}

	if opt.RunAsService {
		if err := s.Run(); err != nil {
			mlog.S().Fatalf("service failed: %v", err)
		}
		os.Exit(0)
	}

	switch opt.Service {
	case "install":
		args := os.Args[1:]
		if len(opt.WorkingDir) == 0 {
			dir, er := os.Getwd()
			if er != nil {
				mlog.S().Fatalf("failed to get current woriking dir: %v", err)
			}
			args = append(args, "--dir", dir)
		}

		args = append(args, "-S") // run as a service
		svcConfig.Arguments = args
		err = s.Install()
	case "uninstall":
		err = s.Uninstall()
	case "start":
		err = s.Start()
	case "stop":
		err = s.Stop()
	case "restart":
		err = s.Restart()
	default:
		mlog.S().Fatalf("unknown service action [%s]", opt.Service)
	}
	if err != nil {
		mlog.S().Fatalf("%s: %v", opt.Service, err)
	} else {
		mlog.S().Infof("%s: done", opt.Service)
		os.Exit(0)
	}
}

func cd() {
	var d string
	switch {
	case opt.CD2Exe: // cd2exe has higher priority.
		execPath, err := os.Executable()
		if err != nil {
			mlog.S().Fatalf("failed to get the executable path: %v", err)
		}
		d = filepath.Dir(execPath)
	case len(opt.WorkingDir) > 0:
		d = opt.WorkingDir
	}

	if len(d) != 0 {
		if err := os.Chdir(d); err != nil {
			mlog.S().Fatalf("failed to change the current working directory: %v", err)
		}
		mlog.S().Infof("changed the working directory to %s", d)
	}
}

type svc struct{}

func (m *svc) Start(s service.Service) error {
	go run()
	return nil
}

func (m *svc) Stop(s service.Service) error {
	return nil
}

func run() {
	if len(opt.LogFile) > 0 {
		f, err := os.OpenFile(opt.LogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0755)
		if err != nil {
			mlog.S().Fatalf("cannot open log file: %v", err)
		}
		fLocked := zapcore.Lock(f)
		mlog.ErrWriter().Replace(fLocked)
		mlog.InfoWriter().Replace(fLocked)
	}

	mlog.S().Infof("mosdns-cn ver: %s", version)
	mlog.S().Infof("arch: %s, os: %s, go: %s", runtime.GOARCH, runtime.GOOS, runtime.Version())

	entry, err := initEntry()
	if err != nil {
		mlog.S().Fatalf("failed to init entry, %v", err)
	}
	h := &dns_handler.DefaultHandler{
		Logger: mlog.L().Named("dns_handler"),
		Entry:  entry,
	}

	// start servers

	if len(opt.ServerAddr) == 0 {
		mlog.S().Fatal("missing server address")
	}
	s := server.Server{
		DNSHandler: h,
		Logger:     mlog.L().Named("server"),
	}
	udpConn, err := net.ListenPacket("udp", opt.ServerAddr)
	if err != nil {
		mlog.S().Fatalf("failed to listen on udp socket, %v", err)
	}
	mlog.S().Infof("listening on udp socket %s", udpConn.LocalAddr())
	l, err := net.Listen("tcp", opt.ServerAddr)
	if err != nil {
		mlog.S().Fatalf("failed to listen on tcp socket, %v", err)
	}
	mlog.S().Infof("listening on tcp socket %s", l.Addr())
	go func() {
		err := s.ServeUDP(udpConn)
		if err != nil {
			mlog.S().Fatalf("udp server exited: %v", err)
		}
	}()
	go func() {
		err := s.ServeTCP(l)
		if err != nil {
			mlog.S().Fatalf("tcp server exited: %v", err)
		}
	}()

	mlog.S().Info("server started")
	select {}
}

// some plugin args require file name start with `ext:`
func addFilePrefix(ss []string) []string {
	o := make([]string, 0, len(ss))
	for _, s := range ss {
		o = append(o, "ext:"+s)
	}
	return o
}

func initEntry() (handler.ExecutableChainNode, error) {
	route := make([]handler.Executable, 0)

	if len(opt.Hosts) > 0 {
		p, err := hosts.Init(handler.NewBP("hosts", hosts.PluginType), &hosts.Args{Hosts: addFilePrefix(opt.Hosts)})
		if err != nil {
			return nil, fmt.Errorf("failed to init hosts, %w", err)
		}
		route = append(route, p.(handler.Executable))
	}

	if len(opt.Arbitrary) > 0 {
		p, err := arbitrary.Init(handler.NewBP("arbitrary", arbitrary.PluginType), &arbitrary.Args{RR: addFilePrefix(opt.Arbitrary)})
		if err != nil {
			return nil, fmt.Errorf("failed to init arbitrary, %w", err)
		}
		route = append(route, p.(handler.Executable))
	}

	if len(opt.BlacklistDomain) > 0 {
		mixMatcher, err := loadDomainMatcher(opt.BlacklistDomain)
		if err != nil {
			return nil, fmt.Errorf("failed to init blacklist, %w", err)
		}
		e := &blackList{m: msg_matcher.NewQNameMatcher(mixMatcher)}
		mlog.S().Infof("black domain files loaded, total length: %d", mixMatcher.Len())
		route = append(route, e)
	}

	if opt.CacheSize > 0 || len(opt.RedisCache) > 0 {
		p, err := cache.Init(handler.NewBP("cache", cache.PluginType), &cache.Args{
			Size:              opt.CacheSize,
			Redis:             opt.RedisCache,
			LazyCacheTTL:      opt.LazyCacheTTL,
			LazyCacheReplyTTL: opt.LazyCacheReplyTTL,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to init cache, %w", err)
		}
		route = append(route, p.(handler.Executable))
	}

	// init upstream
	if len(opt.Upstream) > 0 {
		args, err := initFastForwardArgs(opt.Upstream)
		if err != nil {
			return nil, fmt.Errorf("failed to parse upstream, %w", err)
		}
		p, err := fastforward.Init(handler.NewBP("upstream", fastforward.PluginType), args)
		if err != nil {
			return nil, fmt.Errorf("failed to init upstream, %w", err)
		}
		route = append(route, p.(handler.Executable))
	} else {
		if len(opt.LocalUpstream) == 0 {
			return nil, errors.New("missing local upstream")
		}
		if len(opt.RemoteUpstream) == 0 {
			return nil, errors.New("missing remote upstream")
		}
		if len(opt.LocalIP) == 0 {
			return nil, errors.New("missing local ip")
		}

		var localFastForward handler.Executable
		var remoteFastForward handler.Executable

		var localIPMatcher handler.Matcher
		var localDomainMatcher handler.Matcher
		var remoteDomainMatcher handler.Matcher

		// init local upstream
		args, err := initFastForwardArgs(opt.LocalUpstream)
		if err != nil {
			return nil, fmt.Errorf("failed to parse local upstream, %w", err)
		}
		p, err := fastforward.Init(handler.NewBP("local_upstream", fastforward.PluginType), args)
		if err != nil {
			return nil, fmt.Errorf("failed to init local upstream, %w", err)
		}
		localFastForward = p.(handler.Executable)

		// init remote upstream
		args, err = initFastForwardArgs(opt.RemoteUpstream)
		if err != nil {
			return nil, fmt.Errorf("failed to parse remote upstream, %w", err)
		}
		p, err = fastforward.Init(handler.NewBP("remote_upstream", fastforward.PluginType), args)
		if err != nil {
			return nil, fmt.Errorf("failed to init remote upstream, %w", err)
		}
		remoteFastForward = p.(handler.Executable)

		if len(opt.LocalIP) > 0 {
			nl := netlist.NewList()
			if err := netlist.BatchLoadFromFiles(nl, opt.LocalIP); err != nil {
				return nil, fmt.Errorf("failed to load local ip file, %w", err)
			}
			nl.Sort()
			mlog.S().Infof("local ip files loaded, total length: %d", nl.Len())
			localIPMatcher = msg_matcher.NewAAAAAIPMatcher(nl)
		}

		if len(opt.LocalDomain) > 0 {
			matcher, err := loadDomainMatcher(opt.LocalDomain)
			if err != nil {
				return nil, fmt.Errorf("failed to load local domain file, %w", err)
			}
			mlog.S().Infof("local domain files loaded, total length: %d", matcher.Len())
			localDomainMatcher = msg_matcher.NewQNameMatcher(matcher)
		}

		if len(opt.RemoteDomain) > 0 {
			matcher, err := loadDomainMatcher(opt.RemoteDomain)
			if err != nil {
				return nil, fmt.Errorf("failed to load remote domain file, %w", err)
			}
			mlog.S().Infof("remote domain files loaded, total length: %d", matcher.Len())
			remoteDomainMatcher = msg_matcher.NewQNameMatcher(matcher)
		}

		// forward non A/AAAA query to local upstream.
		m := executable_seq.NagateMatcher(msg_matcher.NewQTypeMatcher(elem.NewIntMatcher([]int{1, 28})))
		innerNode := handler.WrapExecutable(localFastForward)
		innerNode.LinkNext(handler.WrapExecutable(&end{}))
		node := &executable_seq.IfNode{
			ConditionMatcher: m,
			ExecutableNode:   innerNode,
		}
		route = append(route, node)

		// forward local domain to local upstream.
		if localDomainMatcher != nil {
			innerNode := handler.WrapExecutable(localFastForward)
			innerNode.LinkNext(handler.WrapExecutable(&end{}))
			node := &executable_seq.IfNode{
				ConditionMatcher: localDomainMatcher,
				ExecutableNode:   innerNode,
			}
			route = append(route, node)
		}

		// forward remote domain to remote upstream.
		if remoteDomainMatcher != nil {
			innerNode := handler.WrapExecutable(remoteFastForward)
			innerNode.LinkNext(handler.WrapExecutable(&end{}))
			node := &executable_seq.IfNode{
				ConditionMatcher: remoteDomainMatcher,
				ExecutableNode:   innerNode,
			}
			route = append(route, node)
		}

		// distinguish local domain by ip
		primaryRoot := handler.WrapExecutable(localFastForward)
		primaryIf := &executable_seq.IfNode{
			ConditionMatcher: executable_seq.NagateMatcher(localIPMatcher),
			ExecutableNode:   handler.WrapExecutable(&dropResponse{}),
		}
		primaryRoot.LinkNext(primaryIf)

		localLatency := opt.LocalLatency
		if localLatency <= 0 {
			localLatency = 50
		}
		c := &executable_seq.FallbackConfig{
			Primary:       primaryRoot,
			Secondary:     handler.WrapExecutable(remoteFastForward),
			FastFallback:  localLatency,
			AlwaysStandby: true,
		}
		fallbackNode, err := executable_seq.ParseFallbackNode(c, mlog.L())
		if err != nil {
			return nil, fmt.Errorf("inner err, failed to init fallback node, %w", err)
		}
		route = append(route, fallbackNode)
	}

	p, err := ttl.Init(handler.NewBP("ttl", ttl.PluginType), &ttl.Args{
		MaximumTTL: opt.MaxTTL,
		MinimalTTL: opt.MinTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init ttl, %w", err)
	}
	route = append(route, p.(handler.Executable))

	ii := make([]interface{}, 0, len(route))
	for _, node := range route {
		ii = append(ii, node)
	}
	entry, err := executable_seq.ParseExecutableNode(ii, mlog.L())
	if err != nil {
		return nil, fmt.Errorf("inner err, failed to init entry, %w", err)
	}

	load_cache.GetCache().Purge()
	debug.FreeOSMemory()
	return entry, nil
}

func parseFastUpstream(s string) (*fastforward.UpstreamConfig, error) {
	if !strings.Contains(s, "://") {
		s = "udp://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	v := u.Query()
	u.RawQuery = ""
	uc := &fastforward.UpstreamConfig{
		Addr:               u.String(),
		DialAddr:           v.Get("netaddr"),
		Socks5:             v.Get("socks5"),
		EnableHTTP3:        v.Get("enable_http3") == "true",
		EnablePipeline:     v.Get("enable_pipeline") == "true",
		MaxConns:           4,
		InsecureSkipVerify: opt.Insecure,
	}
	idt := 0
	if s := v.Get("keepalive"); len(s) != 0 {
		i, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("invalid keepalive arg, %w", err)
		}
		idt = i
	}
	uc.IdleTimeout = idt

	return uc, nil
}

func initFastForwardArgs(upstreams []string) (*fastforward.Args, error) {
	ua := new(fastforward.Args)
	for i, s := range upstreams {
		uc, err := parseFastUpstream(s)
		if err != nil {
			return nil, fmt.Errorf("invalid upstream address [%s], %w", s, err)
		}
		if i == 0 {
			uc.Trusted = true
		}
		ua.Upstream = append(ua.Upstream, uc)
	}
	ua.CA = opt.CA
	return ua, nil
}

func loadDomainMatcher(files []string) (*domain.MixMatcher, error) {
	mixMatcher := domain.NewMixMatcher(domain.WithDomainMatcher(domain.NewSimpleDomainMatcher()))
	if err := domain.BatchLoadMatcherFromFiles(mixMatcher, files, nil); err != nil {
		return nil, err
	}
	return mixMatcher, nil
}
