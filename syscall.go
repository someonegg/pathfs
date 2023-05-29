package pathfs

import "bytes"

func listXAttr(path string) (attributes []string, err error) {
	dest := make([]byte, 0)
	sz, err := listXAttrSyscall(path, dest)
	if err != nil {
		return nil, err
	}

	for sz > cap(dest) && err == nil {
		dest = make([]byte, sz)
		sz, err = listXAttrSyscall(path, dest)
	}

	if sz == 0 {
		return nil, err
	}

	// -1 to drop the final empty slice.
	dest = dest[:sz-1]
	attributesBytes := bytes.Split(dest, []byte{0})
	attributes = make([]string, len(attributesBytes))
	for i, v := range attributesBytes {
		attributes[i] = string(v)
	}
	return attributes, nil
}

func getXAttr(path string, attr string) ([]byte, error) {
	dest := make([]byte, 0)
	sz, err := getXAttrSyscall(path, attr, dest)

	for sz > cap(dest) && err == nil {
		dest = make([]byte, sz)
		sz, err = getXAttrSyscall(path, attr, dest)
	}

	if err != nil {
		return nil, err
	}

	return dest[:sz], nil
}
