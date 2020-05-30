package config

import (
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/tlahdekorpi/archivegen/elf"
)

const rechars = "*?|({[^$"

func resplit(p string) (dir, re, next string) {
	var i, j, s int
	if i = strings.IndexAny(p, rechars); i == -1 {
		return p, "", ""
	}
	if s = strings.LastIndexByte(p[:i], '/'); s == -1 {
		if ps := strings.IndexByte(p[i:], '/'); ps != -1 {
			return "./", p[:ps+i], p[ps+i+1:]
		} else {
			return "./", p, ""
		}
	}
	if s == 0 {
		dir = "/"
	} else {
		dir = p[:s]
	}
	if j = strings.IndexByte(p[s+1:], '/'); j != -1 {
		re, next = p[s+1:s+j+1], p[s+j+2:]
	} else {
		re = p[s+1:]
	}
	return
}

type elem struct {
	p  string
	n  bool
	re *regexp.Regexp
}

func re(p string) ([]elem, error) {
	var e []elem
	var pa, r, rest string
	for {
		var n bool
		pa, r, rest = resplit(p)
		if len(r) > 0 && r[0] == '!' {
			n = true
			r = r[1:]
		}
		re, err := regexp.Compile(r)
		if err != nil {
			return nil, err
		}
		e = append(e, elem{
			p: pa, n: n, re: re,
		})
		p = rest
		if rest == "" {
			break
		}
	}
	return e, nil
}

func (m *Map) ls(p string, re *regexp.Regexp, dir, ne bool) ([]string, error) {
	rp, err := elf.Expand(p, m.prefix)
	if err != nil {
		return nil, nil
	}

	f, err := os.Open(rp)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	fs, err := f.Stat()
	if err != nil {
		return nil, nil
	}
	if !fs.IsDir() {
		return []string{""}, nil
	}

	n, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	var r []string
	for _, v := range n {
		var m bool
		if ne {
			m = !re.MatchString(v)
		} else {
			m = re.MatchString(v)
		}
		if !m {
			continue
		}
		vs, err := os.Lstat(path.Join(rp, v))
		if err != nil {
			continue
		}
		if id := vs.IsDir(); id {
			r = append(r, v)
		} else if !id && dir {
			r = append(r, v)
		}
	}
	return r, nil
}

func (m *Map) rematch(r []elem, prefix string, f []string) (next []string, err error) {
	if len(r) == 0 {
		return f, nil
	}

	lr := len(r) == 1
	e := r[0]

	for _, v := range f {
		dir := path.Join(prefix, v, e.p)
		n, err := m.ls(dir, e.re, lr, e.n)
		if err != nil {
			return nil, err
		}
		if len(n) == 0 {
			continue
		}
		for _, p := range n {
			next = append(next, path.Join(dir, p))
		}
	}
	return m.rematch(r[1:], "", next)
}

func (m *Map) match(p string) ([]string, error) {
	if strings.IndexAny(p, rechars) == -1 {
		return []string{p}, nil
	}

	r, err := re(p)
	if err != nil {
		return nil, err
	}

	lr := len(r) == 1
	e := r[0]

	f, err := m.ls(e.p, e.re, lr, e.n)
	if err != nil {
		return nil, err
	}

	if lr {
		for k, v := range f {
			f[k] = path.Join(e.p, v)
		}
		return f, nil
	}
	return m.rematch(r[1:], e.p, f)
}
