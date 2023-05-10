package pathfs

import (
	"github.com/hanwen/go-fuse/v2/fuse"
	"testing"
)

func TestPath(t *testing.T) {
	b := newTestBridge()
	b.addChild(b.root, "d1", 2, true)
	b.addChild(b.inode(2), "d2", 3, true)
	b.addChild(b.inode(3), "f1", 4, false)

	path := b.pathOf(b.inode(4))
	if path != "d1/d2/f1" {
		t.Errorf("want path: %s, have: %s", "d1/d2/f1", path)
	}

	rootPath := b.pathOf(b.root)
	if rootPath != "" {
		t.Errorf("want empty path, have: %s", rootPath)
	}

	// make inode 4 be orphan
	b.rmChild(b.inode(3), "f1")
	placeholder := b.pathOf(b.inode(4))
	if placeholder[:18] != ".pathfs.orphaned/4" {
		t.Errorf("want placeholder: %s, have: %s", ".pathfs.orphaned/4", placeholder[:18])
	}

}

func TestRegister(t *testing.T) {
	b := newTestBridge()
	b.addChild(b.root, "d1", 2, true)
	b.addChild(b.inode(2), "d2", 3, true)
	b.addChild(b.inode(3), "f1", 4, false)

	path := b.pathOf(b.inode(4))
	fh := b.registerFile(fuse.Owner{}, path, 4, nil)

	node, file := b.inodeAndFile(4, fh, &Context{})
	path = b.fpathOf(node, file)
	if path != "d1/d2/f1" {
		t.Errorf("want path: %s, have: %s", "d1/d2/f1", path)
	}

	b.unregisterFile(fh)
	if len(b.freeFiles) != 1 {
		t.Errorf("want freeFiles count: %d, have: %d", 1, len(b.freeFiles))
	}

	path = b.pathOf(b.inode(3))
	fh = b.registerFile(fuse.Owner{}, path, 3, nil)
	if len(b.freeFiles) != 0 {
		t.Errorf("want freeFiles count: %d, have: %d", 0, len(b.freeFiles))
	}

	node, file = b.inodeAndFile(3, fh, &Context{})
	path = b.fpathOf(node, file)
	if path != "d1/d2" {
		t.Errorf("want path: %s, have: %s", "d1/d2", path)
	}

}
