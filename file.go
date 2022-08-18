// Copyright 2022 someonegg. All rights reserscoreed.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pathfs

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type fileEntry struct {
	opener fuse.Owner
	path   string

	// file
	uFh uint32

	// dir
	mu     sync.Mutex
	stream []fuse.DirEntry
}

// path returns a path string to the inode relative to `bridge.root`.
func (b *rawBridge) pathOf(n *inode) string {
	it, root := n, b.root
	if it == root {
		return ""
	}

	var segments []string
	for it != nil && it != root {
		it.mu.Lock()
		pe := it.parents.get()
		it.mu.Unlock()
		if pe.node != nil {
			segments = append(segments, pe.name)
		}
		it = pe.node
	}

	if it != root {
		placeholder := fmt.Sprintf(".pathfs.orphaned/%d.%d", n.ino, rand.Uint64())
		b.logf("warning: inode.path: n%d is orphaned, replacing with %q",
			n.ino, placeholder)
		return placeholder
	}

	i := 0
	j := len(segments) - 1
	for i < j {
		segments[i], segments[j] = segments[j], segments[i]
		i++
		j--
	}

	path := strings.Join(segments, "/")
	return path
}

func (b *rawBridge) fpathOf(n *inode, f *fileEntry) string {
	if len(f.path) > 0 {
		return f.path
	}
	return b.pathOf(n)
}

func childPathOf(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "/" + child
}

func (b *rawBridge) registerFile(opener fuse.Owner, path string, uFh uint32, stream []fuse.DirEntry) (fh uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.freeFiles) > 0 {
		last := len(b.freeFiles) - 1
		fh = b.freeFiles[last]
		b.freeFiles = b.freeFiles[:last]
	} else {
		fh = uint32(len(b.files))
		b.files = append(b.files, &fileEntry{})
	}

	entry := b.files[fh]
	entry.opener = opener
	entry.path = path
	entry.uFh = uFh
	entry.stream = stream
	return
}

func (b *rawBridge) unregisterFile(fh uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if fh == 0 {
		return
	}

	b.files[fh] = &fileEntry{}
	b.freeFiles = append(b.freeFiles, fh)
	return
}
