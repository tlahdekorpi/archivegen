package config

import (
	"errors"
	"fmt"
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
	Path PathVar `desc:"Search path"`
}

const (
	TypeOmit         = "-"
	TypeDirectory    = "d"
	TypeRecursive    = "R"
	TypeRecursiveRel = "Rr"
	TypeRegular      = "f"
	TypeRegularRel   = "fr"
	TypeGlob         = "g"
	TypeGlobRel      = "gr"
	TypeSymlink      = "l"
	TypeCreate       = "c"
	TypeCreateNoEndl = "cl"
	TypeLinked       = "L"
	TypeLinkedGlob   = "gL"
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
	mm     maskMap
	libs   []string
	e      Entry
	rootfs *string
	src    string
	err    error
}

type Map struct {
	// overlapping entries will be
	// replaced by subsequent entries.
	A []Entry

	// current set of masks.
	mm maskMap

	// lookup existance/index of entries.
	m map[string]int

	// variable map
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

func multi(s string) []string {
	i := strings.IndexByte(s, '{')
	if i < 0 {
		return []string{s}
	}

	e := strings.LastIndexByte(s[i:], '}')
	if e < 0 {
		return []string{s}
	}

	a, b := s[:i], s[e+i+1:]

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

func lookup(rootfs *string, t string, files ...string) (file string, err error) {
	switch t {
	case TypeLinkedAbs, TypeGlob, TypeRecursive, TypeRegular:
		rootfs = nil
	}
	for _, file = range files {
		_, err = os.Stat(rootPrefix(file, rootfs))
		if err == nil {
			break
		}
	}
	return
}

func (m *Map) add(e entry, rootfs *string, fail bool, line int) error {
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
	mu := multi(e[idx])

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
				mu = multi(e[idx])
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

			err := m.add(e, rootfs, fail, line)
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

	// entry rootfs takes priority
	if r := e.Root(); r != nil {
		rootfs = r
	}

	a := alternate(E.Src)
	if a != nil {
		E.Src, err = lookup(rootfs, e.Type(), a...)
		if !e.isSet(idxDst) {
			E.Dst = clean(E.Src)
		}
	}
	if fail {
		if a == nil {
			_, err = lookup(rootfs, e.Type(), E.Src)
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
		E.Src = rootPrefix(E.Src, rootfs)
	}

	switch E.Type {
	case
		TypeLinkedGlob:
		return m.addElfGlob(E, rootfs)
	case
		TypeLibrary:
		return m.addElfLib(E, rootfs)
	case
		TypeLinkedAbs,
		TypeLinked:
		return m.addElf(E, rootfs)
	case
		TypePath:
		return m.addPath(E, rootfs)
	case
		TypeRecursiveRel,
		TypeRecursive:
		return m.addRecursive(
			E,
			e.isSet(idxUser),
			e.isSet(idxGroup),
			rootfs,
		)
	case
		TypeGlob,
		TypeGlobRel:
		return m.addGlob(
			E,
			e.isSet(idxUser),
			e.isSet(idxGroup),
			rootfs,
		)
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

func rootPrefix(file string, rootfs *string) string {
	if rootfs == nil {
		return file
	}
	if *rootfs == "" {
		return file
	}
	return path.Join(*rootfs, file)
}

func trimPrefix(file string, rootfs *string) string {
	if rootfs == nil {
		return file
	}
	if *rootfs == "" {
		return file
	}
	return strings.TrimPrefix(file, *rootfs)
}

var q chan struct{}

func (m *Map) resolve(e Entry, src string, rootfs *string) {
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
			mm:     mm,
			libs:   r,
			e:      e,
			rootfs: rootfs,
			src:    src,
			err:    err,
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
		if r.rootfs != nil && r.e.Type != TypeLinkedAbs {
			r.e.Dst = strings.TrimPrefix(r.src, *r.rootfs)
		} else {
			r.e.Dst = r.src
		}
		r.e.Dst = strings.TrimLeft(r.e.Dst, "/")
	}

	m.Add(Entry{
		r.src,
		r.e.Dst,
		r.e.User,
		r.e.Group,
		0755,
		TypeRegular,
		"", 0, 0, nil, nil,
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
		v, err = m.expand(v, r.rootfs)
		if err != nil {
			return err
		}

		dst := v
		if r.rootfs != nil {
			dst = strings.TrimPrefix(dst, *r.rootfs)
		}
		dst = strings.TrimPrefix(dst, "/")

		m.Add(Entry{
			v,
			dst,
			r.e.User,
			r.e.Group,
			0755,
			TypeRegular,
			"", 0, 0, nil, nil,
		})
	}

	return nil
}

var elfAdded = make(map[string]struct{})

func (m *Map) addElf(e Entry, rootfs *string) error {
	var src string
	if e.Type != TypeLinkedAbs {
		src = rootPrefix(e.Src, rootfs)
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
		if src, err = m.expand(src, rootfs); err != nil {
			return err
		}
	}

	if Opt.ELF.Concurrent {
		m.resolve(e, src, rootfs)
		return nil
	}

	r, err := m.r.Resolve(src, e.LibraryPath...)
	return m.includeElf(&result{
		libs:   r,
		e:      e,
		rootfs: rootfs,
		src:    src,
		err:    err,
	})

}

func (m *Map) Merge(t *Map) error {
	for _, v := range t.A {
		m.Add(v)
	}
	return nil
}

func (m *Map) addRecursive(e Entry, user, group bool, rootfs *string) error {
	var uid, gid *int
	if user {
		uid = &e.User
	}
	if group {
		gid = &e.Group
	}
	return filepath.Walk(e.Src, mapW{m, e, uid, gid, rootfs}.walkFunc)
}

type mapW struct {
	m      *Map
	e      Entry
	uid    *int
	gid    *int
	rootfs *string
}

func intPtr(i *int, d uint32) int {
	if i != nil {
		return *i
	}
	return int(d)
}

func (m mapW) walkFunc(file string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	// archive filepath
	af := strings.TrimPrefix(file, m.e.Src)
	if af == "" {
		return nil
	}

	var rf string
	if m.e.Dst != TypeOmit {
		rf = path.Join(m.e.Dst, af)
	} else {
		rf = path.Clean(af)
	}

	if m.rootfs != nil {
		rf = strings.TrimPrefix(rf, *m.rootfs)
	}
	rf = strings.TrimPrefix(rf, "/")

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("config: recursive: fileinfo not *Stat_t, %#v)", info.Sys())
	}

	if info.IsDir() {
		m.m.Add(Entry{
			rf,
			rf,
			intPtr(m.uid, stat.Uid),
			intPtr(m.gid, stat.Gid),
			mode(info),
			TypeDirectory,
			"", 0, 0, nil, nil,
		})
		return nil
	}

	if info.Mode().IsRegular() {
		m.m.Add(Entry{
			file,
			rf,
			intPtr(m.uid, stat.Uid),
			intPtr(m.gid, stat.Gid),
			mode(info),
			TypeRegular,
			"", 0, 0, nil, nil,
		})
		return nil
	}

	if info.Mode()&os.ModeSymlink != 0 {
		f, err := os.Readlink(file)
		if err != nil {
			return err
		}

		m.m.Add(Entry{
			f,
			rf,
			intPtr(m.uid, stat.Uid),
			intPtr(m.gid, stat.Gid),
			0777,
			TypeSymlink,
			"", 0, 0, nil, nil,
		})
		return nil
	}

	return fmt.Errorf("config: recursive: unknown file: %s", file)
}

func (m *Map) addGlob(e Entry, user, group bool, rootfs *string) error {
	r, err := filepath.Glob(e.Src)
	if err != nil {
		return err
	}
	if Opt.Warn.EmptyGlob && len(r) < 1 {
		log.Printf("emptyglob: %s", e.Src)
		return nil
	}

	x := mapW{m: m, e: e, rootfs: rootfs}
	if user {
		x.uid = &e.User
	}
	if group {
		x.gid = &e.Group
	}

	x.e.Src = ""

	for _, v := range r {
		s, err := os.Lstat(v)
		if err := x.walkFunc(v, s, err); err != nil {
			return err
		}
	}

	return nil
}

func (m *Map) addElfGlob(e Entry, rootfs *string) error {
	src := rootPrefix(e.Src, rootfs)
	r, err := filepath.Glob(src)
	if err != nil {
		return err
	}

	if Opt.Warn.EmptyGlob && len(r) < 1 {
		log.Printf("emptyglob: %s", src)
		return nil
	}

	for _, v := range r {
		e.Src = trimPrefix(v, rootfs)
		e.Dst = clean(e.Src)
		e.Type = TypeRegular

		if m.mm.apply(&e) {
			continue
		}

		if err := m.addElf(e, rootfs); err != nil {
			return err
		}
	}

	return nil
}

func (m *Map) addElfLib(e Entry, rootfs *string) error {
	var err error
	e.Src, err = m.r.Find(e.Src)
	if err != nil {
		return err
	}

	e.Dst = clean(e.Src)
	return m.addElf(e, rootfs)
}

func (m *Map) addPath(e Entry, rootfs *string) error {
	var (
		file string
		err  error
	)

	if e.Src[0] == '/' {
		return m.addElf(e, rootfs)
	}

	for _, v := range Opt.Path {
		file = path.Join(v, e.Src)
		_, err = os.Lstat(rootPrefix(file, rootfs))
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
	return m.addElf(e, rootfs)
}
