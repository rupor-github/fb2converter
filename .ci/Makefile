#!make

MAKEFLAGS += --always-make --no-print-directory

static/dictionaries:
	cd ../static/dictionaries && \
		wget -r -l1 --no-parent -nd -A.pat.txt http://ctan.math.utah.edu/ctan/tex-archive/language/hyph-utf8/tex/generic/hyph-utf8/patterns/txt
	cd ../static/dictionaries && \
		wget -r -l1 --no-parent -nd -A.hyp.txt http://ctan.math.utah.edu/ctan/tex-archive/language/hyph-utf8/tex/generic/hyph-utf8/patterns/txt
	cd ../static/dictionaries && for item in $$(ls *.txt); do gzip $$item; done

static/sentences:
	cd ../static/sentences && \
		curl -L https://api.github.com/repos/neurosnap/sentences/tarball | tar xz --wildcards '*/data/*.json' --strip-components=2
	$(MAKE) _russian-sentences
	cd ../static/sentences && for item in $$(ls *.json); do gzip $$item; done

# https://github.com/Mottl/ru_punkt
# https://github.com/mhq/train_punkt
_russian-sentences:
ifeq ($(wildcard russian.pickle),)
	wget https://github.com/mhq/train_punkt/archive/refs/heads/master.tar.gz -O punkt.tar.gz \
        && mkdir -p punkt && tar --strip-components=1 -xzf punkt.tar.gz -C punkt
	cp punkt/russian.pickle russian.pickle
	rm -rf punkt*
endif
	python3 convert.py | jq "." > ../static/sentences/russian.json
