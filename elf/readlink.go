package elf

// TODO: do not duplicate code

import (
	"errors"
	"os"
	"path"
	"strings"
)

var errTooManyLinks = errors.New("elf: readlink: max symlinks")

const linkMax = 255

func comp(a, b string) int {
	for k := range a {
		if k >= len(b) {
			return k
		}
		if a[k] != b[k] {
			return k
		}
	}
	return len(a)
}

func readlink(s string, n int, c int, prefix string) (string, error) {
	s = path.Clean(s)

	ls := len(s)
	if n > ls {
		n = ls
	}

	lx := s
	ln := n

	if ls != n {
		nn := strings.IndexByte(s[n:], '/')
		if nn < 0 {
			n = ls
		} else {
			n += nn + 1
		}
		lx = s[:n]
		if l := len(lx) - 1; lx[l] == '/' {
			lx = lx[:l]
		}
	}

	if ln == n {
		n = ls
	}

	f, err := os.Lstat(lx)
	if err != nil {
		return s, err
	}

	if f.Mode()&os.ModeSymlink == 0 {
		if ls != ln {
			return readlink(s, n, c, prefix)
		}
		return s, nil
	}

	if c > linkMax {
		return s, errTooManyLinks
	}
	c++

	r, err := os.Readlink(lx)
	if err != nil {
		return s, err
	}

	p := strings.LastIndexByte(lx, '/')

	var np string
	if r[0] == '/' {
		np = path.Join(prefix, r)
	} else {
		np = path.Join(s[:p+1], r)
	}

	np = path.Join(np, s[n:])
	if strings.Contains(r, "..") {
		ln = comp(lx, np)
	}

	if x := strings.IndexByte(r, '/'); x >= 0 {
		if x != 0 {
			return readlink(np, ln, c, prefix)
		}
		return readlink(np, len(prefix)+1, c, prefix)
	}
	return readlink(np, n, c, prefix)
}

func expand(p, prefix string) (string, error) {
	if len(p) == 0 {
		return p, nil
	}

	if !strings.HasPrefix(p, prefix) {
		return p, nil
	}

	var i int
	if p[0] == '/' {
		i++
	}
	if n := len(prefix); n > 0 {
		i = n + 1
	}
	return readlink(p, i, 0, prefix)
}
