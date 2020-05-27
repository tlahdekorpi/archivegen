package config

import (
	"encoding/base64"
	"fmt"
	"path"
	"strconv"
	"strings"
)

const (
	modeSticky = 1 << (iota + 9)
	modeSetgid
	modeSetuid
)

const (
	idxType = iota
	idxSrc
	idxDst
	idxMode
	idxUser
	idxGroup
	idxData
	idxHeredoc
)

type entry []string

// TODO: uint32 uid, gid <-> syscall.Stat_t
type Entry struct {
	Src, Dst    string
	User, Group int
	Mode        int
	Type        string
	Heredoc     string
	Time        int64
	Line        int
	Data        []byte
	LibraryPath []string
}

func (e entry) Type() string {
	return e[idxType]
}

func (e entry) Src() (string, error) {
	switch e.Type() {
	case
		TypeRegularRel,
		TypeRegular:
		if len(e) < 2 {
			break
		}
		return e[1], nil

	case TypeSymlink:
		if len(e) < 3 {
			break
		}
		return e[1], nil

	case
		TypeDirectory,
		TypeRecursive,
		TypeRecursiveRel,
		TypeGlob,
		TypeGlobRel,
		TypeCreate,
		TypeCreateNoEndl,
		TypeBase64,
		TypePath,
		TypeLibrary,
		TypeLinkedAbs,
		TypeLinkedGlob,
		TypeLinked:
		if len(e) < 2 {
			break
		}
		return e[1], nil
	}

	return "", errInvalidEntry
}

func clean(file string) string {
	p := path.Clean(file)
	if p[0] != '/' {
		return p
	}
	return p[1:]
}

func suffix(e entry) string {
	p := path.Clean(e[2])
	if strings.HasSuffix(e[2], "/") {
		_, f := path.Split(e[1])
		p = path.Join(p, f)
	}
	if p[0] != '/' {
		return p
	}
	return p[1:]
}

func (e entry) Dst() (string, error) {
	switch e.Type() {
	case TypeDirectory:
		// invalid entry
		if len(e) < 2 {
			break
		}
		// directory dst is the src
		return clean(e[1]), nil

	case
		TypeRegularRel,
		TypeRegular:
		// invalid entry
		if len(e) < 2 {
			break
		}

		// omitted dst
		if len(e) < 3 {
			return clean(e[1]), nil
		}

		// dst set
		if e[2] != TypeOmit {
			return clean(e[2]), nil
		}

		// explicitly omitted dst
		return clean(e[1]), nil
	case
		TypePath:
		if len(e) > 2 && e[2] != TypeOmit {
			return suffix(e), nil
		}
		return clean(e[1]), nil
	case
		TypeLibrary,
		TypeLinkedAbs,
		TypeLinkedGlob,
		TypeLinked:
		if len(e) > 2 && e[2] != TypeOmit {
			return clean(e[2]), nil
		}
		return clean(e[1]), nil
	case
		TypeSymlink,
		TypeGlob,
		TypeGlobRel:
		if len(e) < 3 {
			break
		}
		return clean(e[2]), nil
	case
		TypeRecursive,
		TypeRecursiveRel:
		if len(e) < 3 {
			return clean(e[1]), nil
		}
		return clean(e[2]), nil

	case
		TypeCreate,
		TypeCreateNoEndl,
		TypeBase64:
		if len(e) < 2 {
			break
		}
		return clean(e[1]), nil

	}

	return "", errInvalidEntry
}

func (e entry) Mode() (int, error) {
	i := e.typeOffset(idxMode)

	if len(e) <= i || e[i] == TypeOmit {
		switch e.Type() {
		case TypeDirectory:
			return 0755, nil
		case TypeSymlink:
			return 0777, nil
		default:
			return 0644, nil
		}
	}

	m, err := strconv.ParseInt(e[i], 8, 0)
	if err != nil {
		return 0, err
	}

	return int(m), nil
}

func (e entry) typeOffset(i int) int {
	switch e.Type() {
	case
		TypeRecursive,
		TypeRecursiveRel,
		TypeGlob,
		TypeGlobRel,
		TypeDirectory,
		TypeCreate,
		TypeCreateNoEndl,
		TypeBase64:
		i--
	}
	return i
}

func (e entry) isSet(i int) bool {
	i = e.typeOffset(i)
	if len(e) <= i || e[i] == TypeOmit {
		return false
	}
	return true
}

func (e entry) parseIndex(i int) (int, error) {
	i = e.typeOffset(i)
	if len(e) <= i || e[i] == TypeOmit {
		return 0, nil
	}
	r, err := strconv.ParseInt(e[i], 10, 0)
	return int(r), err
}

func (e entry) User() (int, error) {
	return e.parseIndex(idxUser)
}

func (e entry) Group() (int, error) {
	return e.parseIndex(idxGroup)
}

func (e entry) pMode() (int, error) {
	if !e.isSet(idxMode) {
		return -1, nil
	}
	return e.Mode()
}

func (e entry) pUser() (int, error) {
	if !e.isSet(idxUser) {
		return -1, nil
	}
	return e.User()
}

func (e entry) pGroup() (int, error) {
	if !e.isSet(idxGroup) {
		return -1, nil
	}
	return e.Group()
}

func (e entry) Data() []byte {
	var end string
	switch e.Type() {
	case TypeCreate:
		end = "\n"
		break
	case TypeCreateNoEndl, TypeBase64:
		break
	default:
		return nil
	}

	i := e.typeOffset(idxData)
	if len(e) <= i {
		return []byte{}
	}

	return []byte(
		strings.TrimLeft(
			e[i],
			" \t",
		) + end,
	)
}

func (e entry) heredoc() string {
	switch e.Type() {
	case TypeCreate, TypeCreateNoEndl, TypeBase64:
		break
	default:
		return ""
	}

	if len(e) < idxHeredoc {
		return ""
	}
	return e[idxHeredoc-1]
}

func (e entry) Root() *string {
	switch e.Type() {
	case
		TypePath,
		TypeLibrary,
		TypeLinkedAbs,
		TypeLinkedGlob,
		TypeLinked:
		break
	default:
		return nil
	}

	// idxData = idxRoot
	if len(e) <= idxData {
		return nil
	}

	idx := e[idxData]
	if idx != TypeOmit {
		return &idx
	}

	return nil
}

func unescape(s string) string {
	return strings.ReplaceAll(s, `\ `, ` `)
}

func escape(s string) string {
	return strings.ReplaceAll(s, ` `, `\ `)
}

func (e entry) Entry() (Entry, error) {
	var (
		r   Entry
		err error
	)

	r.Dst, err = e.Dst()
	if err != nil {
		return r, err
	}

	r.Src, err = e.Src()
	if err != nil {
		return r, err
	}

	r.Dst = unescape(r.Dst)
	r.Src = unescape(r.Src)

	switch e.Type() {
	case
		TypeRecursive,
		TypeRecursiveRel,
		TypeGlob,
		TypeGlobRel:
		break
	default:
		r.Mode, err = e.Mode()
		if err != nil {
			return r, err
		}
	}

	r.User, err = e.User()
	if err != nil {
		return r, err
	}

	r.Group, err = e.Group()
	if err != nil {
		return r, err
	}

	r.Type = e.Type()
	r.Data = e.Data()

	r.Heredoc = e.heredoc()

	return r, nil
}

func (e Entry) Base64() Entry {
	switch e.Type {
	case TypeCreate, TypeCreateNoEndl:
		break
	default:
		return e
	}

	d := make([]byte, base64.StdEncoding.EncodedLen(len(e.Data)))
	base64.StdEncoding.Encode(d, e.Data)
	e.Data = d
	e.Type = TypeBase64
	e.Heredoc = ""
	return e
}

func (e Entry) Format() string {
	switch e.Type {
	case TypeDirectory:
		return fmt.Sprintf("%s\t\t%s\t%04o\t%d\t%d",
			e.Type, escape(e.Dst), e.Mode, e.User, e.Group,
		)

	case TypeCreate, TypeCreateNoEndl, TypeBase64:
		if e.Heredoc == "" {
			return strings.TrimRight(
				fmt.Sprintf("%s\t%s\t\t%04o\t%d\t%d\t%s",
					e.Type, escape(e.Dst), e.Mode, e.User, e.Group, e.Data,
				), "\n",
			)
		}
		return strings.TrimRight(
			fmt.Sprintf("%s\t%s\t\t%04o\t%d\t%d\t<<%s\n",
				e.Type, escape(e.Dst), e.Mode, e.User, e.Group, e.Heredoc,
			), "\n",
		)
	}

	return fmt.Sprintf("%s\t%s\t%s\t%04o\t%d\t%d",
		e.Type, escape(e.Src), escape(e.Dst), e.Mode, e.User, e.Group,
	)
}
