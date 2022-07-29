// Copyright 2022 someonegg. All rights reserscoreed.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pathfs

import (
	"context"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// Context carries opener information in addition to fuse.Context.
//
// When a FUSE request is canceled, the API routine should respond by
// returning the EINTR status code.
type Context struct {
	fuse.Context
	Opener *fuse.Owner // set when manipulating file handle.
}

func (c *Context) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c *Context) Done() <-chan struct{} {
	return c.Cancel
}

func (c *Context) Err() error {
	select {
	case <-c.Cancel:
		return context.Canceled
	default:
		return nil
	}
}

type openerKeyType struct{}

var openerKey openerKeyType

func (c *Context) Value(key interface{}) interface{} {
	if key == openerKey {
		return c.Opener
	}
	return nil
}

var _ = context.Context((*Context)(nil))

func OpenerValue(ctx context.Context) (*fuse.Owner, bool) {
	v, ok := ctx.Value(openerKey).(*fuse.Owner)
	return v, ok
}

func WithOpener(ctx context.Context, opener *fuse.Owner) context.Context {
	return context.WithValue(ctx, openerKey, opener)
}

var contextPool = sync.Pool{
	New: func() interface{} {
		return &Context{}
	},
}

func newContext(cancel <-chan struct{}, caller fuse.Caller) *Context {
	ctx := contextPool.Get().(*Context)
	ctx.Cancel = cancel
	ctx.Caller = caller
	ctx.Opener = nil
	return ctx
}

func releaseContext(ctx *Context) {
	contextPool.Put(ctx)
}
