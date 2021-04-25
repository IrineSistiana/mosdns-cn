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
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/cache"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/msg_matcher"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/pool"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"time"
)

type cnHandler struct {
	cache          cache.DnsCache  // optional
	localDomain    handler.Matcher // optional
	localUpstream  []utils.Upstream
	remoteDomain   handler.Matcher // optional
	remoteUpstream []utils.Upstream

	localIP      *msg_matcher.AAAAAIPMatcher
	localLatency time.Duration
}

func (h *cnHandler) ServeDNS(ctx context.Context, qCtx *handler.Context, w dns_handler.ResponseWriter) {
	q := qCtx.Q()
	var queryKey string
	if h.cache != nil {
		key, err := utils.GetMsgKey(q, 0)
		queryKey = key
		if err != nil {
			mlog.S().Warnf("Failed to get msg cache key: %v", err)
		} else {
			r, err := h.cache.Get(ctx, key)
			if err != nil {
				mlog.S().Warnf("Cache err: %v", err)
			}

			if r != nil {
				r.Id = q.Id
				w.Write(r)
				return
			}
		}
	}

	r, err := h.exchangeDNS(ctx, qCtx)
	if err != nil {
		mlog.S().Warnf("query failed: %v", err)
		return
	}

	if h.cache != nil && len(queryKey) != 0 {
		if r != nil && r.Rcode == dns.RcodeSuccess && len(r.Answer) > 0 {
			if err := h.cache.Store(ctx, queryKey, r, time.Duration(dnsutils.GetMinimalTTL(r))*time.Second); err != nil {
				mlog.S().Warnf("Failed to store cache: %v", err)
			}
		}
	}

	w.Write(r)
}

func (h *cnHandler) exchangeDNS(ctx context.Context, qCtx *handler.Context) (*dns.Msg, error) {
	if h.remoteDomain != nil {
		ok, _ := h.remoteDomain.Match(ctx, qCtx)
		if ok {
			mlog.S().Debugf("[%s] query is remote domain", qCtx)
			return utils.ExchangeParallel(ctx, qCtx, h.remoteUpstream, mlog.L())
		}
	}

	if h.localDomain != nil {
		ok, _ := h.localDomain.Match(ctx, qCtx)
		if ok {
			mlog.S().Debugf("[%s] query is local domain", qCtx)
			return utils.ExchangeParallel(ctx, qCtx, h.localUpstream, mlog.L())
		}
	}

	return h.fallbackExchange(ctx, qCtx)
}

type result struct {
	m   *dns.Msg
	err error
}

func (h *cnHandler) fallbackExchange(ctx context.Context, qCtx *handler.Context) (*dns.Msg, error) {
	timer := pool.GetTimer(h.localLatency)
	defer pool.ReleaseTimer(timer)
	lc := make(chan *result, 1) // buffed chan
	rc := make(chan *result, 1)
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		r, err := utils.ExchangeParallel(childCtx, qCtx, h.localUpstream, mlog.L())
		lc <- &result{m: r, err: err} // won't block, lc is buffed.
	}()

	go func() {
		r, err := utils.ExchangeParallel(childCtx, qCtx, h.remoteUpstream, mlog.L())
		rc <- &result{m: r, err: err}
	}()

	var crc chan *result
	var done int
	for {
		if done >= 2 {
			return nil, errors.New("all upstreams are failed")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			crc = rc
		case res := <-lc:
			done++
			if err := res.err; err != nil {
				mlog.S().Warnf("Local upstream failed: %v", err)
				crc = rc
				continue
			}
			if h.localIP.MatchMsg(res.m) {
				mlog.S().Debugf("[%s] local response contains local ip, accept it", qCtx)
				return res.m, nil
			} else {
				mlog.S().Debugf("[%s] local response of does not contain local ip", qCtx)
				crc = rc
			}
		case res := <-crc:
			done++
			if err := res.err; err != nil {
				mlog.S().Warnf("Remote upstream failed: %v", err)
				continue
			}
			mlog.S().Debugf("[%s] remote response acceptted", qCtx)
			return res.m, nil
		}
	}
}

type trustedUpstreamWrapper struct {
	trusted bool
	fu      *upstream.FastUpstream
}

func (t *trustedUpstreamWrapper) Exchange(qCtx *handler.Context) (*dns.Msg, error) {
	q := qCtx.Q()
	if qCtx.IsTCPClient() {
		return t.fu.ExchangeNoTruncated(q)
	}
	return t.fu.Exchange(q)
}

func (t *trustedUpstreamWrapper) Address() string {
	return t.fu.Address()
}

func (t *trustedUpstreamWrapper) Trusted() bool {
	return t.trusted
}

func wrapFU(u *upstream.FastUpstream, trusted bool) *trustedUpstreamWrapper {
	return &trustedUpstreamWrapper{trusted: trusted, fu: u}
}
