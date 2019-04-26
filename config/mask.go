package config

import (
	"errors"
	"regexp"
	"strconv"
)

const (
	maskMode      = "mm"
	maskClear     = "mc"
	maskIgnore    = "mi"
	maskIgnoreNeg = "mI"
	maskReplace   = "mr"
)

const (
	idxMaskID     = 1
	idxMaskRegexp = 2
	idxMaskDst    = 3
	idxMaskMode   = 3
	idxMaskUid    = 4
	idxMaskGid    = 5
)

var (
	errInvalidIndex = errors.New("mask: invalid index")
	errIndexOOB     = errors.New("mask: index out of bounds")
)

type maskFunc func(*Entry) bool

type maskMap []maskFunc

func (m maskMap) apply(e *Entry) bool {
	for _, v := range m {
		if v(e) {
			return true
		}
	}
	return false
}

func (m maskMap) set(e entry) (maskMap, error) {
	f, err := maskFromEntry(e)
	if err != nil {
		return nil, err
	}

	if e[idxMaskID] == TypeOmit {
		return append(m, f), nil
	}

	var i int
	if i, err = maskID(e); err != nil {
		return nil, err
	}

	if i >= len(m) {
		return nil, errInvalidIndex
	}

	m[i] = f
	return m, nil
}

func (m maskMap) del(e entry) (maskMap, error) {
	if len(e) < idxMaskID+1 {
		return m[:0], nil
	}

	if e[idxMaskID] == TypeOmit {
		if len(m) < 1 {
			return m, nil
		}
		return m[:len(m)-1], nil
	}

	i, err := maskID(e)
	if err != nil {
		return nil, err
	}

	if i >= len(m) {
		return nil, errIndexOOB
	}

	return append(m[:i], m[i+1:]...), nil
}

func maskID(e entry) (int, error) {
	i, err := strconv.Atoi(e[idxMaskID])
	if err != nil {
		return 0, err
	}
	if i < 0 {
		return 0, errInvalidIndex
	}
	return i, nil
}

func maskFromEntry(e entry) (maskFunc, error) {
	if len(e) < 2 {
		return nil, errInvalidEntry
	}

	// trim preceding / for convenience
	if e[idxMaskRegexp][0] == '/' {
		e[idxMaskRegexp] = e[idxMaskRegexp][1:]
	}

	switch e[idxType] {
	case maskReplace:
		return regexReplaceMask(e)
	case maskMode:
		return regexModeMask(e)
	case maskIgnore:
		return regexIgnoreMask(e, false)
	case maskIgnoreNeg:
		return regexIgnoreMask(e, true)
	}
	return nil, errInvalidEntry
}

func regexReplaceMask(e entry) (maskFunc, error) {
	if len(e) < idxMaskDst {
		return nil, errInvalidEntry
	}

	r, err := regexp.Compile(e[idxMaskRegexp])
	if err != nil {
		return nil, err
	}

	return func(E *Entry) bool {
		switch E.Type {
		case
			TypeLinked,
			TypeLinkedGlob,
			TypeRecursive:
			return false
		}
		E.Dst = r.ReplaceAllString(E.Dst, e[idxMaskDst])
		return false
	}, nil
}

func regexIgnoreMask(e entry, neg bool) (maskFunc, error) {
	if len(e) < idxMaskDst {
		return nil, errInvalidEntry
	}

	r, err := regexp.Compile(e[idxMaskRegexp])
	if err != nil {
		return nil, err
	}

	return func(E *Entry) bool {
		switch E.Type {
		case
			TypeLinked,
			TypeLinkedGlob,
			TypeRecursive:
			return false
		}
		if neg {
			return !r.MatchString(E.Dst)
		}
		return r.MatchString(E.Dst)
	}, nil
}

func regexModeMask(e entry) (maskFunc, error) {
	if len(e) < idxMaskDst {
		return nil, errInvalidEntry
	}

	r, err := regexp.Compile(e[idxMaskRegexp])
	if err != nil {
		return nil, err
	}

	var (
		mode *int
		uid  *int
		gid  *int
	)

	if mode, err = e.pMode(); err != nil {
		return nil, err
	}

	if uid, err = e.pUser(); err != nil {
		return nil, err
	}

	if gid, err = e.pGroup(); err != nil {
		return nil, err
	}

	return func(E *Entry) bool {
		switch E.Type {
		case
			TypeLinked,
			TypeLinkedGlob,
			TypeRecursive:
			return false
		}
		if !r.MatchString(E.Dst) {
			return false
		}
		if mode != nil {
			E.Mode = *mode
		}
		if gid != nil {
			E.Group = *gid
		}
		if uid != nil {
			E.User = *uid
		}
		return false
	}, nil
}
