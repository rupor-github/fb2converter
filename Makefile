#!make

MAKEFLAGS += --always-make --no-print-directory
CALL_PARAM=$(filter-out $@,$(MAKECMDGOALS))

.PHONY: help

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

%:
	@:

########################################################################################################################

owner:
	sudo chown --changes -R $$(whoami):$$(whoami) .
	@echo "Success"

misc/version:
ifeq ($(wildcard misc/version.go),)
	mkdir -p misc && cp cmake/version.go.in misc/version.go
	sed -i "s/@PRJ_VERSION_MAJOR@.@PRJ_VERSION_MINOR@.@PRJ_VERSION_PATCH@/$${GITHUB_REF##*/}-$${GOOS}-$${GOARCH}/g" misc/version.go
	sed -i "s/@GIT_HASH@/$${GITHUB_SHA}/g" misc/version.go
	cat misc/version.go
endif

static/dictionaries:
ifeq ($(shell ls static/dictionaries | grep .gz | tail -1),)
	cd static/dictionaries && \
		wget -r -l1 --no-parent -nd -A.pat.txt http://ctan.math.utah.edu/ctan/tex-archive/language/hyph-utf8/tex/generic/hyph-utf8/patterns/txt
	cd static/dictionaries && \
		wget -r -l1 --no-parent -nd -A.hyp.txt http://ctan.math.utah.edu/ctan/tex-archive/language/hyph-utf8/tex/generic/hyph-utf8/patterns/txt
	cd static/dictionaries && for item in $$(ls *.txt); do gzip $$item; done
endif

static/sentences:
ifeq ($(shell ls static/sentences | grep .gz | tail -1),)
	cd static/sentences && \
		curl -L https://api.github.com/repos/neurosnap/sentences/tarball | tar xz --wildcards '*/data/*.json' --strip-components=2
	cd static/sentences && for item in $$(ls *.json); do gzip $$item; done
endif

build/fb2c:
	@mkdir -p build
	CGO_ENABLED=0 go build -o build/fb2c cmd/fb2c/*.go
	chmod +x build/fb2c

build/fb2epub:
	@mkdir -p build
	CGO_ENABLED=0 go build -o build/fb2epub cmd/fb2epub/*.go
	chmod +x build/fb2epub

build/fb2mobi:
	@mkdir -p build
	CGO_ENABLED=0 go build -o build/fb2mobi cmd/fb2mobi/*.go
	chmod +x build/fb2mobi

build:
	make misc/version
	make static/dictionaries
	make static/sentences
	make build/fb2c
	make build/fb2epub
	make build/fb2mobi
