#!/bin/sh -e

# remove dicts
# rm -f *.gz

# download dicts
wget --recursive --level=1 --no-parent --quiet --show-progress --no-directories --accept=.pat.txt,.hyp.txt http://ctan.math.utah.edu/ctan/tex-archive/language/hyph-utf8/tex/generic/hyph-utf8/patterns/txt

# compress dicts
for a in $(ls *.txt); do gzip --force $a; done

# show result
ls --color=never -sh --ignore=build.sh --ignore=_todo
