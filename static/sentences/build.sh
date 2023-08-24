#!/bin/sh -e

# remove dicts
# rm -f *.gz

# download dicts
curl -L https://api.github.com/repos/neurosnap/sentences/tarball | tar xz --wildcards '*/data/*.json' --strip-components=2

# compress dicts
for a in $(ls *.json); do gzip --force $a; done

# show result
ls --color=never -sh --ignore=build.sh --ignore=_todo
