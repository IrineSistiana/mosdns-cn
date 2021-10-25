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
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/matcher/elem"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/matcher/msg_matcher"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/matcher/netlist"
	_ "github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/server"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/plugin/executable/arbitrary"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/plugin/executable/cache"
	fastforward "github.com/IrineSistiana/mosdns/v2/dispatcher/plugin/executable/fast_forward"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/plugin/executable/hosts"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/plugin/executable/ttl"
	"github.com/jessevdk/go-flags"
	"github.com/kardianos/service"
	"go.uber.org/zap"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

var version = "dev/unknown"

var Opts struct {
	ServerAddr        string   `short:"s" long:"server" description:"Server address"`
	CacheSize         int      `short:"c" long:"cache" description:"Cache size"`
	LazyCacheTTL      int      `long:"lazy-cache-ttl" description:"Responses will stay in the cache for configured seconds."`
	LazyCacheReplyTTL int      `long:"lazy-cache-reply-ttl" description:"TTL value to use when replying with expired data."`
	MinTTL            uint32   `long:"min-ttl" description:"Minimum TTL value for DNS responses"`
	MaxTTL            uint32   `long:"max-ttl" description:"Maximum TTL value for DNS responses"`
	Hosts             []string `long:"hosts" description:"Hosts"`
	Arbitrary         []string `long:"arbitrary" description:"Arbitrary record"`
	BlacklistDomain   []string `long:"blacklist-domain" description:"Blacklist domain"`

	// simple forwarder
	Upstream []string `long:"upstream" description:"Upstream"`

	// local/remote forwarder
	LocalUpstream  []string `long:"local-upstream" description:"Local upstream"` // required if Upstream is empty
	LocalIP        []string `long:"local-ip" description:"Local ip"`
	LocalDomain    []string `long:"local-domain" description:"Local domain"`
	LocalLatency   int      `long:"local-latency" description:"Local latency in milliseconds" default:"50"`
	RemoteUpstream []string `long:"remote-upstream" description:"Remote upstream"` // required if Upstream is empty
	RemoteDomain   []string `long:"remote-domain" description:"Remote domain"`

	Debug        bool     `short:"v" long:"debug" description:"Verbose log"`
	LogFile      string   `long:"log-file" description:"Write logs to a file"`
	WorkingDir   string   `long:"dir" description:"Working dir"`
	CD2Exe       bool     `long:"cd2exe" description:"Change working dir to executable automatically"`
	Service      string   `long:"service" description:"Service control" choice:"install" choice:"uninstall" choice:"start" choice:"stop" choice:"restart"`
	RunAsService bool     `short:"S" description:"Run as a system service" hidden:"true"`
	CA           []string `long:"ca" description:"CA files"`
	Insecure     bool     `long:"insecure" description:"Disable TLS certificate validation"`
}

func main() {
	_, err := flags.Parse(&Opts)
	if err != nil { // error msg has been printed by flags
		os.Exit(1)
	}

	if Opts.Debug {
		mlog.Level().SetLevel(zap.DebugLevel)
	} else {
		mlog.Level().SetLevel(zap.InfoLevel)
	}

	if Opts.CD2Exe {
		execPath, err := os.Executable()
		if err != nil {
			mlog.S().Fatalf("failed to get the executable path: %v", err)
		}
		wd := filepath.Dir(execPath)
		if err := os.Chdir(wd); err != nil {
			mlog.S().Fatalf("failed to change the current working directory: %v", err)
		}
		mlog.S().Infof("current working directory is %s", wd)
	}

	if len(Opts.Service) == 0 && !Opts.RunAsService {
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

	if Opts.RunAsService {
		if err := s.Run(); err != nil {
			mlog.S().Fatalf("service failed: %v", err)
		}
		os.Exit(0)
	}

	switch Opts.Service {
	case "install":
		args := os.Args[1:]
		if len(Opts.WorkingDir) == 0 {
			args = append(args, "--cd2exe")
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
		mlog.S().Fatalf("unknown service action [%s]", Opts.Service)
	}
	if err != nil {
		mlog.S().Fatalf("%s: %v", Opts.Service, err)
	} else {
		mlog.S().Infof("%s: done", Opts.Service)
		os.Exit(0)
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
	if len(Opts.LogFile) > 0 {
		f, err := os.OpenFile(Opts.LogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0755)
		if err != nil {
			mlog.S().Fatalf("can not open log file: %v", err)
		}
		io.MultiWriter()
		mlog.Writer().Replace(f)
	}

	mlog.S().Infof("mosdns-cn ver: %s", version)
	mlog.S().Infof("arch: %s, os: %s, go: %s", runtime.GOARCH, runtime.GOOS, runtime.Version())

	entry, err := initEntry()
	if err != nil {
		mlog.S().Fatalf("failed to init entry, %v", err)
	}
	h := &dns_handler.DefaultHandler{
		Logger: mlog.L(),
		Entry:  entry,
	}

	// start servers
	udpServer := server.NewServer("udp", Opts.ServerAddr, server.WithHandler(h))
	tcpServer := server.NewServer("tcp", Opts.ServerAddr, server.WithHandler(h))
	go func() {
		err := udpServer.Start()
		if err != nil {
			mlog.S().Fatalf("udp server exited: %v", err)
		}
	}()
	go func() {
		err := tcpServer.Start()
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

	if len(Opts.Hosts) > 0 {
		p, err := hosts.Init(handler.NewBP("hosts", hosts.PluginType), &hosts.Args{Hosts: addFilePrefix(Opts.Hosts)})
		if err != nil {
			return nil, fmt.Errorf("failed to init hosts, %w", err)
		}
		route = append(route, p.(handler.Executable))
	}

	if len(Opts.Arbitrary) > 0 {
		p, err := arbitrary.Init(handler.NewBP("arbitrary", arbitrary.PluginType), &arbitrary.Args{RR: addFilePrefix(Opts.Arbitrary)})
		if err != nil {
			return nil, fmt.Errorf("failed to init arbitrary, %w", err)
		}
		route = append(route, p.(handler.Executable))
	}

	if len(Opts.BlacklistDomain) > 0 {
		mixMatcher, err := loadDomainMatcher(Opts.BlacklistDomain)
		if err != nil {
			return nil, fmt.Errorf("failed to init blacklist, %w", err)
		}
		e := &blackList{m: msg_matcher.NewQNameMatcher(mixMatcher)}
		route = append(route, e)
	}

	if s := Opts.CacheSize; s > 8 {
		p, err := cache.Init(handler.NewBP("cache", cache.PluginType), &cache.Args{
			Size:              s,
			LazyCacheTTL:      Opts.LazyCacheTTL,
			LazyCacheReplyTTL: Opts.LazyCacheReplyTTL,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to init cache, %w", err)
		}
		route = append(route, p.(handler.Executable))
	}

	// init upstream
	if len(Opts.Upstream) > 0 {
		args, err := initFastForwardArgs(Opts.Upstream)
		if err != nil {
			return nil, fmt.Errorf("failed to parse upstream, %w", err)
		}
		p, err := fastforward.Init(handler.NewBP("upstream", fastforward.PluginType), args)
		if err != nil {
			return nil, fmt.Errorf("failed to init upstream, %w", err)
		}
		route = append(route, p.(handler.Executable))
	} else {
		if len(Opts.LocalUpstream) == 0 {
			return nil, errors.New("missing local upstream")
		}
		if len(Opts.RemoteUpstream) == 0 {
			return nil, errors.New("missing remote upstream")
		}
		if len(Opts.LocalIP) == 0 {
			return nil, errors.New("missing local ip")
		}

		var localFastForward handler.Executable
		var remoteFastForward handler.Executable

		var localIPMatcher handler.Matcher
		var localDomainMatcher handler.Matcher
		var remoteDomainMatcher handler.Matcher

		// init local upstream
		args, err := initFastForwardArgs(Opts.LocalUpstream)
		if err != nil {
			return nil, fmt.Errorf("failed to parse local upstream, %w", err)
		}
		p, err := fastforward.Init(handler.NewBP("local_upstream", fastforward.PluginType), args)
		if err != nil {
			return nil, fmt.Errorf("failed to init local upstream, %w", err)
		}
		localFastForward = p.(handler.Executable)

		// init remote upstream
		args, err = initFastForwardArgs(Opts.RemoteUpstream)
		if err != nil {
			return nil, fmt.Errorf("failed to parse remote upstream, %w", err)
		}
		p, err = fastforward.Init(handler.NewBP("remote_upstream", fastforward.PluginType), args)
		if err != nil {
			return nil, fmt.Errorf("failed to init remote upstream, %w", err)
		}
		remoteFastForward = p.(handler.Executable)

		if len(Opts.LocalIP) > 0 {
			nl := netlist.NewList()
			if err := netlist.BatchLoadFromFiles(nl, Opts.LocalIP); err != nil {
				return nil, fmt.Errorf("failed to load local ip file, %w", err)
			}
			nl.Sort()
			localIPMatcher = msg_matcher.NewAAAAAIPMatcher(nl)
		}

		if len(Opts.LocalDomain) > 0 {
			matcher, err := loadDomainMatcher(Opts.LocalDomain)
			if err != nil {
				return nil, fmt.Errorf("failed to load local domain file, %w", err)
			}
			localDomainMatcher = msg_matcher.NewQNameMatcher(matcher)
		}

		if len(Opts.RemoteDomain) > 0 {
			matcher, err := loadDomainMatcher(Opts.RemoteDomain)
			if err != nil {
				return nil, fmt.Errorf("failed to load remote domain file, %w", err)
			}
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

		c := &executable_seq.FallbackConfig{
			Primary:       primaryRoot,
			Secondary:     handler.WrapExecutable(remoteFastForward),
			FastFallback:  Opts.LocalLatency,
			AlwaysStandby: true,
		}
		fallbackNode, err := executable_seq.ParseFallbackNode(c, mlog.L())
		if err != nil {
			return nil, fmt.Errorf("inner err, failed to init fallback node, %w", err)
		}
		route = append(route, fallbackNode)
	}

	p, err := ttl.Init(handler.NewBP("ttl", ttl.PluginType), &ttl.Args{
		MaximumTTL: Opts.MaxTTL,
		MinimalTTL: Opts.MinTTL,
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
	return entry, nil
}

func parseFastUpstream(s string) (addr, dialAddr, socks5 string, idt int, err error) {
	if !strings.Contains(s, "://") {
		s = "udp://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", "", "", 0, err
	}
	v := u.Query()
	dialAddr = v.Get("netaddr")
	socks5 = v.Get("socks5")
	if s := v.Get("keepalive"); len(s) != 0 {
		i, err := strconv.Atoi(s)
		if err != nil {
			return "", "", "", 0, fmt.Errorf("invalid keepalive arg, %w", err)
		}
		idt = i
	}

	u.RawQuery = ""
	addr = u.String()
	return addr, dialAddr, socks5, idt, nil
}

func initFastForwardArgs(upstreams []string) (*fastforward.Args, error) {
	ua := new(fastforward.Args)
	for i, s := range upstreams {
		addr, dialAddr, socks5, idt, err := parseFastUpstream(s)
		if err != nil {
			return nil, fmt.Errorf("invalid upstream address [%s], %w", s, err)
		}

		ua.Upstream = append(ua.Upstream, &fastforward.UpstreamConfig{
			Addr:               addr,
			DialAddr:           dialAddr,
			Trusted:            i == 0, // only first upstream is trusted
			Socks5:             socks5,
			IdleTimeout:        idt,
			InsecureSkipVerify: Opts.Insecure,
		})
	}
	ua.CA = Opts.CA
	return ua, nil
}

func loadDomainMatcher(files []string) (*domain.MixMatcher, error) {
	mixMatcher := domain.NewMixMatcher(domain.WithDomainMatcher(domain.NewSimpleDomainMatcher()))
	if err := domain.BatchLoadMatcherFromFiles(mixMatcher, files, nil); err != nil {
		return nil, err
	}
	return mixMatcher, nil
}
