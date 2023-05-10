package pathfs

import "fmt"

func newTestBridge() *rawBridge {
	b := &rawBridge{
		fs:            DefaultFileSystem(),
		root:          newInode(1, true),
		nodeCountHigh: 1,
	}

	b.nodes = map[uint64]*inode{1: b.root}
	b.root.lookupCount = 1
	b.nodeCountHigh = 1
	b.files = []*fileEntry{{}}

	return b
}

// print the directory tree in the format like "tree" command
func printDirTree(root *inode) {
	var lastInLevel []bool

	printBranch := func(level int, last bool, node *inode) {
		prefix := make([]rune, level*3)
		for i := 0; i < level; i++ {
			if lastInLevel[i] {
				prefix[3*i] = ' '
			} else {
				prefix[3*i] = '│'
			}
			prefix[3*i+1] = ' '
			prefix[3*i+2] = ' '
		}
		var symbol string
		if last {
			symbol = "└─ "
		} else {
			symbol = "├─ "
		}
		fmt.Printf("%s%s%s(%d)\n", string(prefix), symbol, node.parents.newest.name, node.ino)
	}

	var walk func(node *inode, level int)
	walk = func(node *inode, level int) {
		if level >= len(lastInLevel) {
			lastInLevel = append(lastInLevel, false)
		}
		last := false
		cnt := 0
		for _, child := range node.children {
			cnt++
			last = cnt == len(node.children)
			lastInLevel[level] = last
			printBranch(level, last, child)
			// is dir
			if child.children != nil {
				walk(child, level+1)
			}
		}
	}

	fmt.Printf(".(%d)\n", root.ino)
	walk(root, 0)

}
