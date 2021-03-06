package config

import (
	"errors"
	"os"
	"path"
	"strings"
)

var errTooManyLinks = errors.New("config: readlink: max symlinks")

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

func (m *Map) readlink(s string, n int, c int) (string, error) {
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
			return m.readlink(s, n, c)
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
		np = path.Join(m.prefix, r)
	} else {
		np = path.Join(s[:p+1], r)
	}

	np = path.Join(np, s[n:])
	if strings.Contains(r, "..") {
		ln = comp(lx, np)
	}

	lx = strings.TrimPrefix(lx, m.prefix)
	if lx[0] == '/' {
		lx = lx[1:]
	}

	m.Add(Entry{
		r,
		lx,
		0,
		0,
		0777,
		TypeSymlink,
		"", 0, 0, nil, nil,
	})

	if x := strings.IndexByte(r, '/'); x >= 0 {
		if x != 0 {
			return m.readlink(np, ln, c)
		}
		return m.readlink(np, len(m.prefix)+1, c)
	}
	return m.readlink(np, n, c)
}

func (m *Map) expand(p string) (string, error) {
	if len(p) == 0 {
		return p, nil
	}

	if !strings.HasPrefix(p, m.prefix) {
		return p, nil
	}

	var i int
	if p[0] == '/' {
		i++
	}
	if n := len(m.prefix); n > 0 {
		i = n + 1
	}
	return m.readlink(p, i, 0)
}
