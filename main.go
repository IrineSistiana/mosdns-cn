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
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/arbitrary"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/cache/mem_cache"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/hosts"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/msg_matcher"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/netlist"
	_ "github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/upstream"
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
	"time"
)

var version = "dev/unknown"

var Opts struct {
	ServerAddr      string   `short:"s" long:"server" description:"Server address"`
	CacheSize       int      `short:"c" long:"cache" description:"Cache size"`
	Hosts           []string `long:"hosts" description:"Hosts"`
	Arbitrary       []string `long:"arbitrary" description:"Arbitrary record"`
	BlacklistDomain []string `long:"blacklist-domain" description:"Blacklist domain"`

	// simple forwarder
	Upstream []string `long:"upstream" description:"Upstream"`

	// local/remote forwarder
	LocalUpstream  []string `long:"local-upstream" description:"Local upstream"` // required if Upstream is empty
	LocalIP        []string `long:"local-ip" description:"Local ip"`
	LocalDomain    []string `long:"local-domain" description:"Local domain"`
	LocalLatency   int      `long:"local-latency" description:"Local latency in milliseconds" default:"50"`
	RemoteUpstream []string `long:"remote-upstream" description:"Remote upstream"` // required if Upstream is empty
	RemoteDomain   []string `long:"remote-domain" description:"Remote domain"`

	Debug        bool   `short:"v" long:"debug" description:"Verbose log"`
	LogFile      string `long:"log-file" description:"Write logs to a file"`
	WorkingDir   string `long:"dir" description:"Working dir"`
	CD2Exe       bool   `long:"cd2exe" description:"Change working dir to executable automatically"`
	Service      string `long:"service" description:"Service control" choice:"install" choice:"uninstall" choice:"start" choice:"stop" choice:"restart"`
	RunAsService bool   `short:"S" description:"Run as a system service" hidden:"true"`
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

	mlog.S().Infof("mosdns ver: %s", version)
	mlog.S().Infof("arch: %s, os: %s, go: %s", runtime.GOARCH, runtime.GOOS, runtime.Version())

	h, err := initHandler()
	if err != nil {
		mlog.S().Fatal(err)
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

func initHandler() (*cnHandler, error) {
	h := new(cnHandler)

	if Opts.CacheSize > 8 {
		h.cache = mem_cache.NewMemCache(8, Opts.CacheSize/8, time.Minute)
	}

	if len(Opts.Hosts) > 0 {
		hs, err := hosts.NewHostsFromFiles(Opts.Hosts)
		if err != nil {
			mlog.S().Fatalf("failed to init hosts: %v", err)
		}
		h.hosts = hs
	}

	if len(Opts.Arbitrary) > 0 {
		a := arbitrary.NewArbitrary()

		if err := a.BatchLoadFiles(Opts.Arbitrary); err != nil {
			mlog.S().Fatalf("failed to init arbitrary: %v", err)
		}
		h.arbitrary = a
	}

	if len(Opts.BlacklistDomain) > 0 {
		loadDomainMatcher("blacklist", Opts.BlacklistDomain, &h.blacklistDomainMatcher)
	}

	for i, s := range Opts.Upstream {
		fu, err := initFastUpstream(s)
		if err != nil {
			mlog.S().Fatalf("failed to init upstream #%d: %v", i, err)
		}

		trusted := false
		if i == 0 {
			trusted = true
		}
		h.upstream = append(h.upstream, wrapFU(fu, trusted))
	}

	// check args
	if len(h.upstream) > 0 {
		return h, nil // This simple forward mode. Skip the followings.
	}

	if len(Opts.LocalUpstream) == 0 {
		return nil, errors.New("missing local upstream")
	}
	if len(Opts.RemoteUpstream) == 0 {
		return nil, errors.New("missing remote upstream")
	}
	if len(Opts.LocalIP) == 0 {
		return nil, errors.New("missing local ip")
	}

	for i, s := range Opts.LocalUpstream {
		fu, err := initFastUpstream(s)
		if err != nil {
			mlog.S().Fatalf("failed to init local upstream #%d: %v", i, err)
		}

		trusted := false
		if i == 0 {
			trusted = true
		}
		h.localUpstream = append(h.localUpstream, wrapFU(fu, trusted))
	}

	if len(Opts.LocalIP) > 0 {
		nl := netlist.NewList()
		if err := netlist.BatchLoadFromFiles(nl, Opts.LocalIP); err != nil {
			mlog.S().Fatalf("failed to load local ip: %v", err)
		}
		nl.Sort()
		mlog.S().Infof("local IP matcher loaded, length: %d", nl.Len())
		h.localIPMatcher = msg_matcher.NewAAAAAIPMatcher(nl)
	}

	if len(Opts.LocalDomain) > 0 {
		loadDomainMatcher("local", Opts.LocalDomain, &h.localDomainMatcher)
	}

	h.localLatency = time.Millisecond * time.Duration(Opts.LocalLatency)

	for i, s := range Opts.RemoteUpstream {
		fu, err := initFastUpstream(s)
		if err != nil {
			mlog.S().Fatalf("failed to init remote upstream #%d: %v", i, err)
		}

		trusted := false
		if i == 0 {
			trusted = true
		}
		h.remoteUpstream = append(h.remoteUpstream, wrapFU(fu, trusted))
	}

	if len(Opts.RemoteDomain) > 0 {
		loadDomainMatcher("remote", Opts.RemoteDomain, &h.remoteDomainMatcher)
	}

	return h, nil
}

func initFastUpstream(s string) (*upstream.FastUpstream, error) {
	if !strings.Contains(s, "://") {
		s = "udp://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream address, %w", err)
	}
	v := u.Query()
	u.RawQuery = ""
	opts := make([]upstream.Option, 0)
	opts = append(opts, upstream.WithDialAddr(v.Get("netaddr")), upstream.WithSocks5(v.Get("socks5")))
	if s := v.Get("keepalive"); len(s) != 0 {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("invalid upstream keepalive arg, %w", err)
		}
		opts = append(opts, upstream.WithIdleTimeout(time.Duration(n)*time.Second))
	}

	return upstream.NewFastUpstream(u.String(), opts...)
}

func loadDomainMatcher(name string, files []string, to **msg_matcher.QNameMatcher) {
	mixMatcher := domain.NewMixMatcher(domain.WithDomainMatcher(domain.NewSimpleDomainMatcher()))
	if err := domain.BatchLoadMatcherFromFiles(mixMatcher, files, nil); err != nil {
		mlog.S().Fatalf("failed to load %s domain: %v", name, err)
	}
	mlog.S().Infof("%s domain matcher loaded, length: %d", name, mixMatcher.Len())
	*to = msg_matcher.NewQNameMatcher(mixMatcher)
}
