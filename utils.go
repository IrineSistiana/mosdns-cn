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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/msg_matcher"
	"github.com/miekg/dns"
)

type blackList struct {
	m *msg_matcher.QNameMatcher
}

func (b *blackList) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	q := qCtx.Q()
	if b.m.MatchMsg(q) {
		r := new(dns.Msg)
		r.SetReply(q)
		r.Rcode = dns.RcodeNameError
		qCtx.SetResponse(r, handler.ContextStatusRejected)
		return nil
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}

type end struct{}

func (e *end) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	return nil
}

type dropResponse struct{}

func (d *dropResponse) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	qCtx.SetResponse(nil, handler.ContextStatusDropped)
	return handler.ExecChainNode(ctx, qCtx, next)
}
