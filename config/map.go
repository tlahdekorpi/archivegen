package config

import (
	"errors"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	stdelf "debug/elf"

	"github.com/tlahdekorpi/archivegen/elf"
)

type PathVar []string

func (p *PathVar) String() string {
	return ""
}

func (p *PathVar) Set(v string) error {
	*p = strings.Split(v, ":")
	return nil
}

var Opt struct {
	Warn struct {
		EmptyGlob bool `desc:"Glob types don't return any matches"`
		Replace   bool `desc:"Entry is replaced"`
	}
	ELF struct {
		Expand       bool `desc:"Resolve all ELF source symlinks"`
		Fallback     bool `desc:"Fallback to adding a file on ELF format errors"`
		Once         bool `desc:"Only add ELFs once"`
		Concurrent   bool `desc:"Load ELFs concurrently, results are added to the end of the archive"`
		NumGoroutine int  `desc:"Number of goroutines resolving ELFs"`
	}
	Glob struct {
		Expand bool `desc:"Resolve all symlinks for globs"`
	}
	File struct {
		Expand bool `desc:"Resolve all symlinks for relative files"`
	}
	Path PathVar `desc:"Search path"`
}

const (
	TypeOmit         = "-"
	TypeDirectory    = "d"
	TypeRecursive    = "R"
	TypeRecursiveRel = "Rr"
	TypeRegular      = "f"
	TypeRegularRel   = "fr"
	TypeGlob         = "r"
	TypeGlobRel      = "rr"
	TypeSymlink      = "l"
	TypeCreate       = "c"
	TypeCreateNoEndl = "cl"
	TypeLinked       = "L"
	TypeLinkedGlob   = "rL"
	TypeLinkedAbs    = "LA"
	TypeLibrary      = "i"
	TypePath         = "p"
	TypeBase64       = "b64"
	TypeVariable     = "$"
)

type variable struct {
	value string
	flag  bool
}

type variableMap struct {
	m map[string]variable
	r *strings.Replacer
}

func newVariableMap(vars []string) *variableMap {
	if len(vars)&0x1 != 0 {
		panic("invalid vars")
	}

	ret := &variableMap{
		m: make(map[string]variable),
		r: strings.NewReplacer(),
	}

	if vars == nil {
		return ret
	}

	r := make([]string, 0)
	for x := 0; x < len(vars); x += 2 {
		ret.m[vars[x]] = variable{
			flag:  true,
			value: vars[x+1],
		}

		r = append(r, TypeVariable+vars[x], vars[x+1])
	}

	ret.r = strings.NewReplacer(r...)

	return ret
}

func (m *variableMap) add(e entry) error {
	if len(e) < 2 {
		return errInvalidEntry
	}

	// variables with flag variable are not mutable
	if v, ok := m.m[e[idxSrc]]; v.flag && ok {
		return nil
	}

	if len(e) > 2 {
		m.m[e[idxSrc]] = variable{value: e[idxDst]}
	} else {
		// without dst assume empty string
		// should this be TypeOmit?
		m.m[e[idxSrc]] = variable{}
	}

	r := make([]string, 0, len(m.m))
	for k, v := range m.m {
		r = append(r, TypeVariable+k, v.value)
	}
	m.r = strings.NewReplacer(r...)

	return nil
}

type result struct {
	mm   maskMap
	libs []string
	e    Entry
	src  string
	err  error
}

type Map struct {
	// overlapping entries will be
	// replaced by subsequent entries.
	A []Entry

	// current set of masks.
	mm maskMap

	// lookup existance/index of entries.
	m map[string]int

	v *variableMap
	r *elf.Resolver

	prefix string

	wg  sync.WaitGroup
	mu  sync.Mutex
	elf []*result
}

func (c *Config) newMap() *Map {
	return &Map{
		m:      make(map[string]int),
		mm:     make(maskMap, 0),
		A:      make([]Entry, 0),
		v:      newVariableMap(c.Vars),
		prefix: c.Prefix,
		r:      c.Resolver,
	}
}

func multi(s string, escape bool) []string {
	i := strings.IndexByte(s, '{')
	if i < 0 {
		return []string{s}
	}

	// TODO: a better solution for regex and multi overlap
	if escape && i > 0 && s[i-1] != '\\' {
		return []string{s}
	}

	e := strings.LastIndexByte(s[i:], '}')
	if e < 0 {
		return []string{s}
	}

	a, b := s[:i], s[e+i+1:]
	if escape {
		a = a[:len(a)-1]
	}

	var r []string
	for _, v := range strings.Split(s[i+1:i+e], ",") {
		r = append(r, a+v+b)
	}

	return r
}

func alternate(s string) []string {
	i := strings.IndexByte(s, '(')
	if i < 0 {
		return nil
	}

	e := strings.LastIndexByte(s[i:], ')')
	if e < 0 {
		return nil
	}

	a, b := s[:i], s[e+i+1:]

	var r []string
	for _, v := range strings.Split(s[i+1:i+e], "|") {
		r = append(r, a+v+b)
	}

	if len(r) == 1 {
		return nil
	}
	return r
}

func lookup(prefix string, t string, files ...string) (file string, err error) {
	switch t {
	case TypeLinkedAbs, TypeGlob, TypeRecursive, TypeRegular:
		prefix = ""
	}
	for _, file = range files {
		file, err = elf.Expand(path.Join(prefix, file), prefix)
		// TODO: use the absolute path as source and do not trim here.
		file = strings.TrimPrefix(file, prefix)
		if err == nil {
			break
		}
	}
	return
}

func (m *Map) add(e entry, fail bool, line int) error {
	for k, _ := range e {
		e[k] = m.v.r.Replace(e[k])
	}

	var err error
	switch e.Type() {
	case
		maskLibrary,
		maskTime,
		maskReplace,
		maskIgnore,
		maskIgnoreNeg,
		maskMode:
		m.mm, err = m.mm.set(e)
		return err
	case maskClear:
		m.mm, err = m.mm.del(e)
		return err
	case TypeVariable:
		return m.v.add(e)
	}

	idx := idxSrc

	var mu []string
	switch e.Type() {
	case TypeGlob, TypeGlobRel, TypeLinkedGlob:
		mu = multi(e[idx], true)
	default:
		mu = multi(e[idx], false)
	}

	var dst string
	if len(e) > idxDst && e[idxDst] != TypeOmit {
		switch e.Type() {
		case
			TypeDirectory,
			TypeGlob,
			TypeGlobRel,
			TypeCreate,
			TypeCreateNoEndl,
			TypeBase64:
			break
		case TypeSymlink:
			if len(mu) == 1 {
				idx = idxDst
				mu = multi(e[idx], false)
				break
			}
			fallthrough
		default:
			dst = e[idxDst]
		}
	}

	if len(mu) > 1 {
		for _, v := range mu {
			if dst != "" {
				_, x := path.Split(v)
				e[idxDst] = path.Join(dst, x)
			}

			e[idx] = v

			err := m.add(e, fail, line)
			if err != nil {
				return err
			}
		}

		return nil
	}

	E, err := e.Entry()
	E.Line = line
	if err != nil {
		return err
	}

	var a []string
	switch e.Type() {
	case TypeGlob, TypeGlobRel, TypeLinkedGlob:
	default:
		a = alternate(E.Src)
	}

	if a != nil {
		E.Src, err = lookup(m.prefix, e.Type(), a...)
		if !e.isSet(idxDst) {
			E.Dst = clean(E.Src)
		}
	}
	if fail {
		if a == nil {
			E.Src, err = lookup(m.prefix, e.Type(), E.Src)
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	if err != nil {
		return err
	}

	if m.mm.apply(&E) {
		// ignored by mask
		return nil
	}

	switch E.Type {
	case TypeRegularRel:
		E.Type = TypeRegular
		fallthrough
	case
		TypeRecursiveRel,
		TypeGlobRel:
		E.Src = path.Join(m.prefix, E.Src)
		if E.Type != TypeRegular || !Opt.File.Expand {
			break
		}
		if E.Src, err = m.expand(E.Src); err != nil {
			return err
		}
		if !e.isSet(idxDst) {
			E.Dst = clean(strings.TrimPrefix(E.Src, m.prefix))
		}
	}

	var uid, gid int
	switch E.Type {
	case
		TypeRecursive,
		TypeRecursiveRel,
		TypeGlob,
		TypeGlobRel:
		if uid, err = e.pUser(); err != nil {
			return err
		}
		if gid, err = e.pGroup(); err != nil {
			return err
		}
	}

	switch E.Type {
	case
		TypeLinkedGlob:
		return m.addElfGlob(E)
	case
		TypeLibrary:
		return m.addElfLib(E)
	case
		TypeLinkedAbs,
		TypeLinked:
		return m.addElf(E)
	case
		TypePath:
		return m.addPath(E)
	case
		TypeRecursiveRel,
		TypeRecursive:
		return m.addRecursive(E, uid, gid)
	case
		TypeGlob,
		TypeGlobRel:
		return m.addGlob(E, uid, gid)
	}

	if i, exists := m.m[E.Dst]; exists {
		rlog(m.A[i], E)
		m.A[i] = E
		return nil
	}

	m.A = append(m.A, E)
	m.m[E.Dst] = len(m.A) - 1
	return nil
}

func (m *Map) Add(e Entry) {
	if m.mm.apply(&e) {
		return
	}

	if i, exists := m.m[e.Dst]; exists {
		rlog(m.A[i], e)
		m.A[i] = e
		return
	}

	m.A = append(m.A, e)
	m.m[e.Dst] = len(m.A) - 1
}

func rlog(e1, e2 Entry) {
	if !Opt.Warn.Replace {
		return
	}
	if e1.Src == e2.Src {
		return
	}
	log.Printf("replace: %s -> %s", e1.Src, e2.Src)
}

var q chan struct{}

func (m *Map) resolve(e Entry, src string) {
	if q == nil {
		if Opt.ELF.NumGoroutine < 1 {
			Opt.ELF.NumGoroutine = 1
		}
		q = make(chan struct{}, Opt.ELF.NumGoroutine)
	}

	mm := make(maskMap, len(m.mm))
	copy(mm, m.mm)

	m.wg.Add(1)
	q <- struct{}{}
	go func() {
		r, err := m.r.Resolve(src, e.LibraryPath...)
		m.mu.Lock()
		m.elf = append(m.elf, &result{
			mm:   mm,
			libs: r,
			e:    e,
			src:  src,
			err:  err,
		})
		m.mu.Unlock()
		m.wg.Done()
		<-q
	}()
}

type multiError []string

func (e multiError) Error() string {
	if len(e) == 1 {
		return e[0]
	}
	return "\n  " + strings.Join(e, "\n  ")
}

func (m *Map) includeElfs() error {
	m.wg.Wait()

	var r multiError
	mm := m.mm
	for _, v := range m.elf {
		m.mm = v.mm
		if err := m.includeElf(v); err != nil {
			r = append(r, lineError{v.e.Line, err}.Error())
		}
	}

	m.mm = mm
	if len(r) == 0 {
		return nil
	}
	return r
}

func (m *Map) includeElf(r *result) error {
	if r.err != nil && !errors.Is(r.err, io.EOF) {
		switch r.err.(type) {
		case *stdelf.FormatError:
		default:
			return r.err
		}
	}

	if strings.TrimLeft(r.e.Src, "/") == r.e.Dst {
		if r.e.Type != TypeLinkedAbs {
			r.e.Dst = strings.TrimPrefix(r.src, m.prefix)
		} else {
			r.e.Dst = r.src
		}
		r.e.Dst = strings.TrimLeft(r.e.Dst, "/")
	}

	m.Add(Entry{
		Src:   r.src,
		Dst:   r.e.Dst,
		User:  r.e.User,
		Group: r.e.Group,
		Mode:  0755,
		Type:  TypeRegular,
	})

	if r.err != nil {
		if Opt.ELF.Fallback {
			return nil
		} else {
			return r.err
		}
	}

	for _, v := range r.libs {
		if v == r.src {
			continue
		}

		var err error
		v, err = m.expand(v)
		if err != nil {
			return err
		}

		m.Add(Entry{
			Src:   v,
			Dst:   clean(strings.TrimPrefix(v, m.prefix)),
			User:  r.e.User,
			Group: r.e.Group,
			Mode:  0755,
			Type:  TypeRegular,
		})
	}

	return nil
}

var elfAdded = make(map[string]struct{})

func (m *Map) addElf(e Entry) error {
	var src string
	if e.Type != TypeLinkedAbs {
		src = path.Join(m.prefix, e.Src)
	} else {
		src = e.Src
	}

	if Opt.ELF.Once {
		if _, ok := elfAdded[src]; ok {
			return nil
		}
		elfAdded[src] = struct{}{}
	}

	var err error
	if Opt.ELF.Expand {
		if src, err = m.expand(src); err != nil {
			return err
		}
	}

	if Opt.ELF.Concurrent {
		m.resolve(e, src)
		return nil
	}

	r, err := m.r.Resolve(src, e.LibraryPath...)
	return m.includeElf(&result{
		libs: r,
		e:    e,
		src:  src,
		err:  err,
	})

}

func (m *Map) Merge(t *Map) error {
	for _, v := range t.A {
		m.Add(v)
	}
	return nil
}

var (
	errModeType = errors.New("config: unsupported filemode")
	errStatType = errors.New("config: invalid stat type")
)

func mode(f os.FileInfo) int {
	m := f.Mode()
	r := m.Perm()

	if m&os.ModeSticky != 0 {
		r |= modeSticky
	}
	if m&os.ModeSetgid != 0 {
		r |= modeSetgid
	}
	if m&os.ModeSetuid != 0 {
		r |= modeSetuid
	}
	return int(r)
}

func idef(i int, d uint32) int {
	if i != -1 {
		return i
	}
	return int(d)
}

func (m *Map) auto(src, dst string, uid, gid int, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errStatType
	}

	e := Entry{
		Src:   src,
		Dst:   clean(dst),
		Mode:  mode(info),
		User:  idef(uid, stat.Uid),
		Group: idef(gid, stat.Gid),
	}

	switch info.Mode() & os.ModeType {
	case 0:
		e.Type = TypeRegular
	case os.ModeDir:
		e.Type = TypeDirectory
	case os.ModeSymlink:
		l, err := os.Readlink(src)
		if err != nil {
			return err
		}
		e.Src = l
		e.Mode = 0777
		e.Type = TypeSymlink
	default:
		return errModeType
	}

	m.Add(e)
	return nil
}

func (m *Map) addRecursive(e Entry, uid, gid int) error {
	src, err := m.expand(e.Src)
	if err != nil {
		return err
	}

	return filepath.Walk(src,
		func(file string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if file == src {
				return nil
			}

			f := strings.TrimPrefix(file, src)
			if e.Dst != TypeOmit {
				f = path.Join(e.Dst, f)
			}
			return m.auto(file, f, uid, gid, info)
		},
	)
}

func (m *Map) addGlob(e Entry, uid, gid int) error {
	p, r, err := m.match(e.Src)
	if err != nil {
		return err
	}

	if Opt.Warn.EmptyGlob && len(r) < 1 {
		log.Printf("emptyglob: %s", e.Src)
		return nil
	}

	for _, v := range r {
		var src string
		if Opt.Glob.Expand && e.Type == TypeGlobRel {
			src, err = m.expand(v)
			if err != nil {
				return err
			}
		} else {
			src = v
		}

		var dst string
		if e.Dst != TypeOmit {
			dst = path.Join(e.Dst, strings.TrimPrefix(v, p))
		} else {
			dst = strings.TrimPrefix(src, m.prefix)
		}

		l, err := os.Lstat(src)
		if err != nil {
			return err
		}
		if err := m.auto(src, dst, uid, gid, l); err != nil {
			return err
		}
	}
	return nil
}

func (m *Map) addElfGlob(e Entry) error {
	_, r, err := m.match(path.Join(m.prefix, e.Src))
	if err != nil {
		return err
	}

	if Opt.Warn.EmptyGlob && len(r) < 1 {
		log.Printf("emptyglob: %s", e.Src)
		return nil
	}

	for _, v := range r {
		e.Src = strings.TrimPrefix(v, m.prefix)
		e.Dst = clean(e.Src)

		if err := m.addElf(e); err != nil {
			return err
		}
	}
	return nil
}

func (m *Map) addElfLib(e Entry) error {
	if src, err := m.r.Find(e.Src); err != nil {
		return err
	} else {
		e.Src = src
		e.Dst = clean(src)
		return m.addElf(e)
	}
}

func (m *Map) addPath(e Entry) error {
	var (
		file string
		err  error
	)

	if e.Src[0] == '/' {
		return m.addElf(e)
	}

	for _, v := range Opt.Path {
		file = path.Join(v, e.Src)
		_, err = os.Lstat(path.Join(m.prefix, file))
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}

	if e.Src == e.Dst {
		e.Dst = clean(file)
	}

	e.Src = file
	return m.addElf(e)
}
