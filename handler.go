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
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/arbitrary"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/cache/mem_cache"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/hosts"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/msg_matcher"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/pool"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/server/dns_handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"time"
)

type cnHandler struct {
	cache                  *mem_cache.MemCache       // optional
	hosts                  *hosts.Hosts              // optional
	arbitrary              *arbitrary.Arbitrary      // optional
	blacklistDomainMatcher *msg_matcher.QNameMatcher // optional
	upstream               []utils.Upstream          // if not empty, cnHandler will act as a simple forwarder.
	localDomainMatcher     *msg_matcher.QNameMatcher // optional
	localUpstream          []utils.Upstream          // required if upstream is empty
	remoteDomainMatcher    *msg_matcher.QNameMatcher // optional
	remoteUpstream         []utils.Upstream          // required if upstream is empty

	localIPMatcher *msg_matcher.AAAAAIPMatcher
	localLatency   time.Duration
}

func (h *cnHandler) ServeDNS(ctx context.Context, qCtx *handler.Context, w dns_handler.ResponseWriter) {

	r, err := h.serveDNS(ctx, qCtx)
	if err != nil {
		mlog.S().Warnf("query failed: %v", err)
		return
	}

	w.Write(r)
}

func (h *cnHandler) serveDNS(ctx context.Context, qCtx *handler.Context) (*dns.Msg, error) {
	q := qCtx.Q()

	if h.hosts != nil {
		if r := h.hosts.LookupMsg(q); r != nil {
			mlog.S().Debugf("[%s] query is responsed by hosts", qCtx)
			return r, nil
		}
	}

	if h.arbitrary != nil {
		if r := h.arbitrary.LookupMsg(q); r != nil {
			mlog.S().Debugf("[%s] query is responsed by arbitrary", qCtx)
			return r, nil
		}
	}

	if h.blacklistDomainMatcher != nil {
		if h.blacklistDomainMatcher.MatchMsg(qCtx.Q()) {
			mlog.S().Debugf("[%s] query is a blacklist domain", qCtx)
			r := new(dns.Msg)
			r.SetReply(qCtx.Q())
			r.Rcode = dns.RcodeNameError
			return r, nil
		}
	}

	var cacheKey string
	if h.cache != nil {
		key, err := utils.GetMsgKey(q, 0)
		cacheKey = key
		if err != nil {
			mlog.S().Warnf("failed to get msg cache key: %v", err)
		} else {
			r, err := h.cache.Get(ctx, key)
			if err != nil {
				mlog.S().Warnf("cache err: %v", err)
			}
			if r != nil {
				mlog.S().Debugf("[%s] cache hit", qCtx)
				r.Id = q.Id
				return r, nil
			}
		}
	}

	r, err := h.exchangeDNS(ctx, qCtx)
	if err != nil {
		return nil, err
	}

	if h.cache != nil && len(cacheKey) != 0 {
		if r != nil && r.Rcode == dns.RcodeSuccess && len(r.Answer) > 0 {
			if err := h.cache.Store(ctx, cacheKey, r, time.Duration(dnsutils.GetMinimalTTL(r))*time.Second); err != nil {
				mlog.S().Warnf("failed to store cache: %v", err)
			}
		}
	}

	return r, nil
}

func (h *cnHandler) exchangeDNS(ctx context.Context, qCtx *handler.Context) (*dns.Msg, error) {
	if len(h.upstream) != 0 { // simple forwarder mode
		return utils.ExchangeParallel(ctx, qCtx, h.upstream, mlog.L())
	}

	if h.localDomainMatcher != nil {
		if h.localDomainMatcher.MatchMsg(qCtx.Q()) {
			mlog.S().Debugf("[%s] query is a local domain", qCtx)
			return utils.ExchangeParallel(ctx, qCtx, h.localUpstream, mlog.L())
		}
	}

	if h.remoteDomainMatcher != nil {
		if h.remoteDomainMatcher.MatchMsg(qCtx.Q()) {
			mlog.S().Debugf("[%s] query is a remote domain", qCtx)
			return utils.ExchangeParallel(ctx, qCtx, h.remoteUpstream, mlog.L())
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
				mlog.S().Warnf("local upstream failed: %v", err)
				crc = rc
				continue
			}
			if h.localIPMatcher.MatchMsg(res.m) {
				mlog.S().Debugf("[%s] local response contains local ip, accept it", qCtx)
				return res.m, nil
			} else {
				mlog.S().Debugf("[%s] local response of does not contain local ip", qCtx)
				crc = rc
			}
		case res := <-crc:
			done++
			if err := res.err; err != nil {
				mlog.S().Warnf("remote upstream failed: %v", err)
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
