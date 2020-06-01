BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
VERSION := $(BRANCH)-$(shell git describe --tags --always --dirty="-dirty")

ifndef PKGS
PKGS := $(shell go list -f '{{if .TestGoFiles}}{{.ImportPath}}{{end}}' ./...)
endif

ifndef BUILDFLAGS
BUILDFLAGS := -i -v
endif

default: archivegen

all: test archivegen

archivegen:
	go build -tags copy_file_range -ldflags "-X main.buildversion=cfr+$(VERSION)" $(BUILDFLAGS)

test:
	go test $(PKGS)

.PHONY: archivegen
