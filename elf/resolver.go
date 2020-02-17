package elf

import (
	"bufio"
	"debug/elf"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var Opt struct {
	LDGlob string `desc:"ld.conf glob"`
}

type errorNotFound string

func (e errorNotFound) Error() string {
	return "resolver: not found: " + string(e)
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
	v := p[s]
	v.class = c
	p[s] = v
}

func (p pathset) list() []pe {
	i := len(p)
	r := make([]pe, i)

	for k, v := range p {
		r[v.id] = pe{k, v.origin, v.class}
	}

	return r
}

func rootprefix(file string, rootfs *string, abs bool) string {
	if abs {
		return file
	}
	if rootfs == nil {
		return file
	}
	if *rootfs == "" {
		return file
	}
	return path.Join(*rootfs, file)
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
	dirs  set
	files set
}

func newfileset() fileset {
	return fileset{
		dirs:  make(set),
		files: make(set),
	}
}

func (f fileset) add(dir string) {
	if f.dirs.add(dir) {
		return
	}

	rdir, err := expand(dir)
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

func (f fileset) ok(dir, file string) bool {
	if f.dirs == nil {
		return true
	}

	f.add(dir)
	_, ok := f.files[file]
	return ok
}

type context struct {
	err    set
	cache  fileset
	ldconf pathset
	class  elf.Class
	root   *string
	abs    bool
}

func (c *context) search1(file string, ret set, from []pe) (string, elfFile, error) {
	var r string

	for _, v := range from {
		if v.class != elf.ELFCLASSNONE && v.class != c.class {
			continue
		}

		dir := v.path
		if file[0] != '/' {
			// relative path
			dir = rootprefix(v.path, c.root, c.abs && v.origin)
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
			if !c.cache.ok(dir, r) {
				continue
			}
		}

		f, err := open(r)
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

		if e, ok := f.(*elf.File); ok {
			if e.Class != c.class {
				c.ldconf.set(v.path, e.Class)
				if err := f.Close(); err != nil {
					return "", nil, err
				}
				continue
			}
		}

		return r, f, nil
	}

	return r, nil, errorNotFound(r)
}

func (c *context) search(file string, ret set, path ...[]pe) (string, elfFile, error) {
	var (
		f   elfFile
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

func (c *context) resolv(file string, f elfFile, rpath pathset, runpath []pe, ret set) error {
	if ret.add(file) {
		return nil
	}

	needed, err := f.DynString(elf.DT_NEEDED)
	if err != nil {
		return err
	}

	oldrunpath := runpath
	var newrunpath []string
	newrunpath, err = f.DynString(elf.DT_RUNPATH)
	if err != nil {
		return err
	}
	runpath = mkpe(split(newrunpath))

	rpathE, err := f.DynString(elf.DT_RPATH)
	if err != nil {
		return err
	}

	// opened in resolve/search
	if err := f.Close(); err != nil {
		return err
	}

	var rd string
	if c.root != nil {
		rd = path.Dir(strings.TrimPrefix(file, *c.root))
	} else {
		rd = path.Dir(file)
	}
	rpath = rpath.add(
		rd,
		split(rpathE)...,
	)

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

func ldglob(glob string, rootfs *string, abs bool) ([]string, error) {
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

var (
	ctxcache     = newfileset()
	resolvecache = make(map[string][]string)

	ldconf       []string
	defaultclass elf.Class
)

func resolve(file string, rootfs *string, abs, cache bool) ([]string, error) {
	file = rootprefix(file, rootfs, abs)
	if r, ok := resolvecache[file]; ok {
		return r, nil
	}

	ctx := context{
		err:   make(set),
		abs:   abs,
		root:  rootfs,
		class: elf.ELFCLASS64,
	}
	if cache {
		ctx.cache = ctxcache
	}

	if cache && ldconf == nil {
		var err error
		if ldconf, err = ldglob(Opt.LDGlob, rootfs, false); err != nil {
			return nil, err
		}
	}

	ctx.ldconf = ctx.ldconf.add(path.Dir(file), ldconf...)

	f, err := open(file)
	if err != nil {
		return nil, err
	}

	if e, ok := f.(*elf.File); ok {
		ctx.class = e.Class
	}

	if defaultclass == elf.ELFCLASSNONE {
		defaultclass = ctx.class
	}

	ret := make(set)
	if i, err := readinterp(f); err == nil {
		ret.add(rootprefix(i, rootfs, false))
	} /* else {
		log.Println("resolve:", err)
	} */

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
			strings.Join(ctx.err.list(), ", "),
		)
	}

	list := ret.list()
	resolvecache[file] = list
	return list, nil
}

// Resolve resolves libraries needed by an ELF file.
func Resolve(file string) ([]string, error) {
	return resolve(file, nil, true, true)
}

// ResolveRoot searches libraries from rootfs. If abs, file will not prefixed with rootfs.
func ResolveRoot(file, rootfs string, abs bool) ([]string, error) {
	return resolve(file, &rootfs, abs, true)
}

func classmatch(file string, class elf.Class) bool {
	f, err := elf.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()
	return class == f.Class
}

// Find searches files with matching class from ld.conf and default paths.
func Find(file string, rootfs *string, class elf.Class) (string, error) {
	if defaultclass == elf.ELFCLASSNONE {
		defaultclass = elf.ELFCLASS64
	}

	if class == elf.ELFCLASSNONE {
		class = defaultclass
	}

	if ldconf == nil {
		if r, err := ldglob(Opt.LDGlob, rootfs, false); err != nil {
			return "", err
		} else {
			ldconf = r
		}
	}

	for _, v := range ldconf {
		p := path.Join(rootprefix(v, rootfs, false), file)
		if classmatch(p, class) {
			return path.Join(v, file), nil
		}
	}

	for _, v := range defaultLibs {
		p := path.Join(rootprefix(v.path, rootfs, false), file)
		if classmatch(p, class) {
			return path.Join(v.path, file), nil
		}
	}

	return "", errorNotFound(file)
}
