#!/usr/bin/python3
# -*- coding: utf-8 -*-

import codecs
import pickle
import io

if __name__ == "__main__":
    with open('russian.pickle', 'rb') as f:
        tokenizer = pickle.load(f)
    p = tokenizer._params

    buf = io.StringIO()

    buf.write('{')

    buf.write('"OrthoContext": {')
    first = True
    for key, value in p.ortho_context.items():
        if not first:
            buf.write(', ')
        else:
            first = False
        buf.write('"{}": {}'.format(key, value))
    buf.write('},')

    buf.write('"Colocations": {')
    first = True
    for (a,b) in p.collocations:
        if not first:
            buf.write(', ')
        else:
            first = False
        buf.write('"{},{}": 1'.format(a,b))
    buf.write('},')

    buf.write('"AbbrevTypes": {')
    first = True
    for a in p.abbrev_types:
        if not first:
            buf.write(', ')
        else:
            first = False
        buf.write('"{}": 1'.format(a))
    buf.write('},')

    buf.write('"SentStarters": {')
    first = True
    for a in p.sent_starters:
        if not first:
            buf.write(', ')
        else:
            first = False
        buf.write('"{}": 1'.format(a))
    buf.write('}')

    buf.write('}')

    print(buf.getvalue())
