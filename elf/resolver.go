package elf

import (
	"bufio"
	"debug/elf"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

type errorNotFound string

func (e errorNotFound) Error() string {
	return "resolve " + string(e) + ": not found"
}

func split(a []string) []string {
	var r []string
	for _, v := range a {
		r = append(r, strings.Split(v, ":")...)
	}
	return r
}

type pt struct {
	id     int
	origin bool
	class  elf.Class
}

type pathset map[string]pt

func (p pathset) copy() pathset {
	r := make(pathset, len(p))
	for k, v := range p {
		r[k] = v
	}
	return r
}

func tokenExpander(origin string) *strings.Replacer {
	return strings.NewReplacer(
		"$ORIGIN", origin,
		"${ORIGIN}", origin,
		"$LIB", "lib64",
		"${LIB}", "lib64",
		"$PLATFORM", "x86_64",
		"${PLATFORM}", "x86_64",
	)
}

func isorigin(s string) bool {
	return strings.Contains(s, "$ORIGIN") || strings.Contains(s, "${ORIGIN}")
}

func (p pathset) add(origin string, s ...string) pathset {
	var (
		sr *strings.Replacer
		c  bool
		r  = p
	)

	for _, v := range s {
		if len(v) < 1 {
			continue
		}
		switch v[0] {
		case '/', '$':
			break
		default:
			continue
		}

		if sr == nil {
			sr = tokenExpander(origin)
		}

		o := isorigin(v)
		v = sr.Replace(v)

		if _, exists := r[v]; exists {
			continue
		}
		if !c {
			r = r.copy()
			c = true
		}
		r[v] = pt{len(r), o, elf.ELFCLASSNONE}
	}

	return r
}

type pe struct {
	path   string
	origin bool
	class  elf.Class
}

func (p pathset) set(s string, c elf.Class) {
	if v, ok := p[s]; ok {
		v.class = c
		p[s] = v
	}
}

func (p pathset) list() []pe {
	i := len(p)
	r := make([]pe, i)

	for k, v := range p {
		r[v.id] = pe{k, v.origin, v.class}
	}

	return r
}

func rootprefix(file string, rootfs string, abs bool) string {
	if abs {
		return file
	}
	return path.Join(rootfs, file)
}

type set map[string]struct{}

func (s set) add(key string) bool {
	if _, exists := s[key]; exists {
		return exists
	}
	s[key] = struct{}{}
	return false
}

func (s set) list() []string {
	var (
		r = make([]string, len(s))
		i int
	)
	for k, _ := range s {
		r[i] = k
		i++
	}

	return r
}

type fileset struct {
	prefix string
	mu     sync.Mutex
	dirs   set
	files  set
}

func newfileset(prefix string) *fileset {
	return &fileset{
		dirs:   make(set),
		files:  make(set),
		prefix: prefix,
	}
}

func (f *fileset) add(dir string) {
	if f.dirs.add(dir) {
		return
	}

	rdir, err := Expand(dir, f.prefix)
	if err != nil && !os.IsNotExist(err) {
		return
	} else {
		rdir = dir
	}

	f.dirs.add(rdir)

	fd, err := os.Open(rdir)
	if err != nil {
		return
	}
	defer fd.Close()

	r, err := fd.Readdirnames(-1)
	if err != nil {
		panic(err)
	}

	for _, v := range r {
		f.files.add(path.Join(dir, v))
		f.files.add(path.Join(rdir, v))
	}
}

func (f *fileset) ok(dir, file string) bool {
	f.mu.Lock()
	if f.dirs == nil {
		f.mu.Unlock()
		return true
	}

	f.add(dir)
	_, ok := f.files[file]
	f.mu.Unlock()
	return ok
}

type context struct {
	err    set
	cache  *fileset
	ldconf pathset
	class  elf.Class
	root   string
	loader Loader
}

func (c *context) search1(file string, ret set, from []pe) (string, ELF, error) {
	var r string

	for _, v := range from {
		if v.class != elf.ELFCLASSNONE && v.class != c.class {
			continue
		}

		dir := v.path
		if file[0] != '/' {
			// relative path
			dir = rootprefix(v.path, c.root, v.origin)
			r = path.Join(dir, file)
		} else {
			// absolute path
			r = file
		}

		_, exists := ret[r]
		if exists {
			return r, nil, nil
		}

		// ignore caching for defaultlibs
		switch v.path {
		case
			"/lib64",
			"/usr/lib64",
			"/lib",
			"/usr/lib":
			break
		default:
			if c.cache != nil && !c.cache.ok(dir, r) {
				continue
			}
		}

		f, err := c.loader(r, c.root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			switch err.(type) {
			case *elf.FormatError:
				continue
			}
			return "", nil, err
		}

		if f.Class() == c.class {
			return r, f, nil
		}

		c.ldconf.set(v.path, f.Class())
		if err := f.Close(); err != nil {
			return "", nil, err
		}
	}

	return r, nil, errorNotFound(r)
}

func (c *context) search(file string, ret set, path ...[]pe) (string, ELF, error) {
	var (
		f   ELF
		r   string
		err error
	)

	for _, v := range path {
		if r, f, err = c.search1(file, ret, v); err == nil {
			return r, f, nil
		}

		switch err.(type) {
		case errorNotFound:
			continue
		default:
			return r, nil, err
		}
	}

	if r, f, err = c.search1(file, ret, c.ldconf.list()); err != nil {
		switch err.(type) {
		case errorNotFound:
			return c.search1(file, ret, defaultLibs)
		default:
			return r, nil, err
		}
	}

	return r, f, err
}

func mkpe(p []string) []pe {
	r := make([]pe, len(p))
	for i := 0; i < len(p); i++ {
		r[i] = pe{p[i], false, elf.ELFCLASSNONE}
	}
	return r
}

func (c *context) resolv(file string, f ELF, rpath pathset, runpath []pe, ret set) error {
	if ret.add(file) {
		return nil
	}

	e, err := f.Dynamic()
	if err != nil {
		return err
	}
	needed := e.Needed

	oldrunpath := runpath
	runpath = mkpe(split(e.Runpath))

	rd := path.Dir(file)
	rpath = rpath.add(rd, split(e.Rpath)...)

	if len(runpath) > 0 {
		x := tokenExpander(rd)
		for k, v := range runpath {
			runpath[k] = pe{
				x.Replace(v.path),
				isorigin(v.path),
				elf.ELFCLASSNONE,
			}
		}
	}

	for _, v := range needed {
		// glibc libc.so is not an elf and
		// in musl it is the interpreter
		if v == "libc.so" {
			break
		}

		s, fd, err := c.search(
			v,
			ret,
			oldrunpath,
			runpath,
			rpath.list(),
		)
		if err != nil {
			switch err.(type) {
			case errorNotFound:
				c.err.add(v)
				continue
			default:
				return err
			}
		}

		if fd == nil {
			continue
		}

		delete(c.err, v)

		if err := c.resolv(s, fd, rpath, runpath, ret); err != nil {
			return err
		}

		ret.add(s)
	}

	return nil
}

func ldglob(glob string, rootfs string, abs bool) ([]string, error) {
	var r []string

	g, err := filepath.Glob(rootprefix(glob, rootfs, abs))
	if err != nil {
		// non-fatal
		return r, nil
	}

	m := make(set)
	for _, v := range g {
		f, err := os.Open(v)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		s := bufio.NewScanner(f)
		for s.Scan() {
			if err := s.Err(); err != nil {
				return nil, err
			}

			t := s.Text()
			if len(t) == 0 || t[0] == '#' {
				continue
			}

			if i := strings.Fields(t); len(i) > 1 {
				if i[0] != "include" {
					continue
				}

				fi := i[1]
				if abs = path.IsAbs(fi); !abs {
					fi = path.Join(path.Dir(v), fi)
				}

				rr, err := ldglob(fi, rootfs, !abs)
				if err != nil {
					return nil, err
				}

				r = append(r, rr...)
				continue
			}

			p := path.Clean(t)
			if !m.add(p) {
				r = append(r, p)
			}
		}
	}

	return r, nil
}

var defaultLibs = mkpe([]string{
	"/lib64",
	"/usr/lib64",
	"/lib",
	"/usr/lib",
})

type File struct {
	Runpath []string
	Rpath   []string
	Needed  []string
}

type ELF interface {
	Interpreter() (string, error)
	Dynamic() (File, error)
	Class() elf.Class
	Close() error
}

type Loader func(file, prefix string) (ELF, error)

type Resolver struct {
	Loader Loader
	prefix string
	ldconf []string
	cache  *fileset
	mu     sync.Mutex
	class  elf.Class
}

func NewResolver(prefix string) *Resolver {
	return &Resolver{
		cache:  newfileset(prefix),
		prefix: prefix,
		Loader: defaultLoader,
	}
}

func (r *Resolver) ReadConfig(file string) error {
	ld, err := ldglob(file, r.prefix, false)
	r.ldconf = ld
	return err
}

func (r *Resolver) Resolve(file string, ld ...string) ([]string, error) {
	f, err := r.Loader(file, r.prefix)
	if err != nil {
		return nil, err
	}

	ctx := context{
		err:    make(set),
		class:  f.Class(),
		root:   r.prefix,
		cache:  r.cache,
		loader: r.Loader,
	}
	ctx.ldconf = ctx.ldconf.add(
		path.Dir(file),
		append(ld, r.ldconf...)...,
	)

	r.mu.Lock()
	if r.class == elf.ELFCLASSNONE {
		r.class = ctx.class
	}
	r.mu.Unlock()

	ret := make(set)
	if i, err := f.Interpreter(); err == nil {
		ret.add(path.Join(r.prefix, i))
	}

	if err := ctx.resolv(
		file,
		f,
		make(pathset),
		nil,
		ret,
	); err != nil {
		return nil, err
	}

	if len(ctx.err) > 0 {
		return nil, errorNotFound(
			file + ": " + strings.Join(ctx.err.list(), ", "),
		)
	}

	return ret.list(), nil
}

func (r *Resolver) classmatch(file string, class elf.Class) bool {
	f, err := r.Loader(file, r.prefix)
	if err != nil {
		return false
	}
	defer f.Close()

	if class == elf.ELFCLASSNONE {
		class = elf.ELFCLASS64
	}
	return class == f.Class()
}

func (r *Resolver) Find(file string) (string, error) {
	for _, v := range r.ldconf {
		p := path.Join(r.prefix, v, file)
		if r.classmatch(p, r.class) {
			return path.Join(v, file), nil
		}
	}

	for _, v := range defaultLibs {
		p := path.Join(r.prefix, v.path, file)
		if r.classmatch(p, r.class) {
			return path.Join(v.path, file), nil
		}
	}
	return "", errorNotFound(file)
}
