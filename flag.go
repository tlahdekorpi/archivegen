package main

import (
	"flag"
	"reflect"
	"strings"

	"github.com/tlahdekorpi/archivegen/config"
)

func set(name, desc string, v reflect.Value) {
	switch x := v.Interface().(type) {
	case *int:
		flag.IntVar(x, name, *x, desc)
	case *string:
		flag.StringVar(x, name, *x, desc)
	case *bool:
		flag.BoolVar(x, name, *x, desc)
	case *config.PathVar:
		flag.Var(x, name, desc)
	default:
		panic("flag")
	}
}

func buildflags(opts interface{}, prefix string) {
	y := reflect.ValueOf(opts).Elem()
	if y.Kind() != reflect.Struct {
		return
	}

	t := y.Type()
	for i := 0; i < y.NumField(); i++ {
		if !y.Field(i).CanSet() {
			continue
		}

		f := t.Field(i)

		n, ok := f.Tag.Lookup("flag")
		if !ok {
			n = prefix + strings.ToLower(f.Name)
		}

		if f.Type.Kind() == reflect.Struct {
			buildflags(y.Field(i).Addr().Interface(), n+".")
		} else {
			set(
				n,
				f.Tag.Get("desc"),
				y.Field(i).Addr(),
			)
		}
	}
}
