// Copyright 2022 someonegg. All rights reserscoreed.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Copyright 2021 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pathfs

import "time"

type parentEntry struct {
	name string
	node *inode
}

// inodeParents stores zero or more parents of an Inode,
// remembering which one is the most recent.
//
// No internal locking: the caller is responsible for preventing
// concurrent access.
type inodeParents struct {
	// newest is the most-recently add()'ed parent.
	// nil when we don't have any parents.
	newest parentEntry
	// other are parents in addition to the newest.
	// nil or empty when we have <= 1 parents.
	other map[parentEntry]time.Time
}

// add adds a parent to the store.
func (p *inodeParents) add(n parentEntry) {
	// one and only parent
	if p.newest.node == nil {
		p.newest = n
	}
	// already known as `newest`
	if p.newest == n {
		return
	}
	// old `newest` gets displaced into `other`
	if p.other == nil {
		p.other = make(map[parentEntry]time.Time)
	}
	p.other[p.newest] = time.Now()
	// new parent becomes `newest` (possibly moving up from `other`)
	delete(p.other, n)
	p.newest = n
}

// get returns the most recent parent
// or nil if there is no parent at all.
func (p *inodeParents) get() parentEntry {
	return p.newest
}

// all returns all known parents
// or nil if there is no parent at all.
func (p *inodeParents) all() []parentEntry {
	count := p.count()
	if count == 0 {
		return nil
	}
	out := make([]parentEntry, 0, count)
	out = append(out, p.newest)
	for i := range p.other {
		out = append(out, i)
	}
	return out
}

func (p *inodeParents) delete(n parentEntry) {
	// We have zero parents, so we can't delete any.
	if p.newest.node == nil {
		return
	}
	// If it's not the `newest` it must be in `other` (or nowhere).
	if p.newest != n {
		delete(p.other, n)
		return
	}
	// We want to delete `newest`, but there is no other to replace it.
	if len(p.other) == 0 {
		p.newest = parentEntry{}
		return
	}
	// Move second newest entry from `other` over `newest`.
	var i parentEntry
	t := time.Time{}
	for k, v := range p.other {
		if t.Before(v) {
			t, i = v, k
		}
	}
	p.newest = i
	delete(p.other, i)
}

func (p *inodeParents) clear() {
	p.newest = parentEntry{}
	p.other = nil
}

func (p *inodeParents) count() int {
	if p.newest.node == nil {
		return 0
	}
	return 1 + len(p.other)
}
