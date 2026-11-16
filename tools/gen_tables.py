#!/usr/bin/env python3
"""Generate internal/pyre/tables.go: the facts of Python's `html`, `re` and `str` that the Go port reproduces.

The module gives the same results as the Python package, so these tables come from the Python standard library rather
than from Go's `unicode` package (which has its own Unicode version):

* the HTML5 named character references and the numeric-reference fixups of `html.unescape`;
* Unicode decimal digits (what Python's `\\d` matches and `int()` reads), `str.isdigit()`, whitespace (`\\s`,
  `str.isspace()`, `str.strip()`) and word characters (`\\w`, `\\b`);
* `str.lower()` and `str.upper()`: the full case mappings, and the Cased / Case_Ignorable sets that decide the final
  sigma;
* `re.IGNORECASE`: the simple lowercase mapping and the "cased" test of the `_sre` module, and the extra case pairs
  (i/ı, s/ſ, µ/μ, ...) of `re._casefix`.

Run from the module root with Python 3.13 (Unicode 15.1, the same tables as the other ports):

    python3.13 tools/gen_tables.py > internal/pyre/tables.go && gofmt -l internal/pyre
"""
import html
import html.entities
import re
import sys
import unicodedata

import _sre

try:
    from re._casefix import _EXTRA_CASES as CASE_FIXES  # Python 3.11+
except ImportError:
    from sre_compile import _ignorecase_fixes as CASE_FIXES

CODEPOINTS = [cp for cp in range(0x110000) if not 0xD800 <= cp <= 0xDFFF]
ALL_CHARS = "".join(map(chr, CODEPOINTS))


def ranges(pred):
    """Inclusive (first, last) runs of code points (surrogates excluded) for which pred(char) is true."""
    out = []
    for cp in CODEPOINTS:
        if pred(chr(cp)):
            if out and out[-1][1] == cp - 1:
                out[-1][1] = cp
            else:
                out.append([cp, cp])
    return [tuple(r) for r in out]


def matching(pattern, flags=0):
    """Code points matched by a one-character pattern."""
    return [ord(m.group()) for m in re.finditer(pattern, ALL_CHARS, flags)]


def go_str(s):
    out = []
    for ch in s:
        o = ord(ch)
        if ch in '\\"':
            out.append("\\" + ch)
        elif 0x20 <= o < 0x7F:
            out.append(ch)
        elif o <= 0xFF and o < 0x80:
            out.append("\\x%02x" % o)
        elif o <= 0xFFFF:
            out.append("\\u%04x" % o)
        else:
            out.append("\\U%08x" % o)
    return '"' + "".join(out) + '"'


def flat(rs):
    return [x for r in rs for x in r]


def int_slice(w, name, values, comment, per_line=8):
    w("// %s\n" % comment)
    w("var %s = []int32{\n" % name)
    for i in range(0, len(values), per_line):
        w("\t" + ", ".join("0x%X" % v for v in values[i:i + per_line]) + ",\n")
    w("}\n\n")


def str_slice(w, name, values, comment, per_line=8):
    w("// %s\n" % comment)
    w("var %s = []string{\n" % name)
    for i in range(0, len(values), per_line):
        w("\t" + ", ".join(go_str(v) for v in values[i:i + per_line]) + ",\n")
    w("}\n\n")


def main():
    # ---- html.unescape ----
    html5 = sorted(html.entities.html5.items())
    assert all("\x00" not in k + v for k, v in html5)
    invalid_charrefs = sorted(html._invalid_charrefs.items())
    invalid_codepoints = sorted(html._invalid_codepoints)

    # ---- \d, \s, \w, isdigit ----
    digits = ranges(lambda c: re.match(r"\d", c) is not None)
    zeros = [z for a, b in digits for z in range(a, b + 1, 10)]
    assert all((b - a + 1) % 10 == 0 for a, b in digits), digits
    assert all(unicodedata.digit(chr(z)) == 0 and unicodedata.digit(chr(z + 9)) == 9 for z in zeros)
    assert digits == ranges(str.isdecimal)
    spaces = ranges(lambda c: re.match(r"\s", c) is not None)
    assert spaces == ranges(str.isspace)
    words = ranges(lambda c: re.match(r"\w", c) is not None)
    assert words == ranges(lambda c: c.isalnum() or c == "_")
    isdigit = ranges(str.isdigit)

    # ---- str.lower(), str.upper() ----
    lower = {}
    upper = {}
    for cp in CODEPOINTS:
        c = chr(cp)
        if cp >= 0x80 and cp != 0x3A3 and c.lower() != c:
            lower[cp] = c.lower()
        if cp >= 0x80 and c.upper() != c:
            upper[cp] = c.upper()
    ascii_chars = "".join(map(chr, range(0x80)))
    assert ascii_chars.lower() == ascii_chars.translate({cp: cp + 32 for cp in range(0x41, 0x5B)})
    assert ascii_chars.upper() == ascii_chars.translate({cp: cp - 32 for cp in range(0x61, 0x7B)})
    # the context-dependent mapping of str.lower() is only the final sigma
    for cp in lower:
        assert ("a" + chr(cp) + "a").lower() == "a" + lower[cp] + "a"
        assert ("." + chr(cp) + " ").lower() == "." + lower[cp] + " "
    for cp in upper:
        assert ("a" + chr(cp) + "a").upper() == "A" + upper[cp] + "A"

    def final_sigma(s):
        return s.lower()[-1] == "ς"

    # str.lower() turns U+03A3 into a final sigma when, skipping case-ignorable characters, a cased character comes
    # before it and none after it. c + "Σ" ends in a final sigma exactly when c is cased and not case-ignorable;
    # "A" + c + "Σ" also does when c is case-ignorable.
    cased = ranges(lambda c: final_sigma(c + "Σ"))
    ignorable = ranges(lambda c: final_sigma("A" + c + "Σ") and not final_sigma(c + "Σ"))

    # ---- re.IGNORECASE (the _sre module) ----
    sre_lower = {cp: _sre.unicode_tolower(cp) for cp in CODEPOINTS if _sre.unicode_tolower(cp) != cp}
    sre_cased = ranges(lambda c: _sre.unicode_iscased(ord(c)))
    fixes = sorted(CASE_FIXES.items())
    # \d, \s and \w are unaffected by lowering the text first (Python checks the lowered character against a class)
    for rs in (digits, spaces, words):
        inside = {cp for a, b in rs for cp in range(a, b + 1)}
        assert all((sre_lower.get(cp, cp) in inside) == (cp in inside) for cp in CODEPOINTS)
    # the case pairs of every lowered character are lowercase themselves, so lowering them again changes nothing
    assert all(sre_lower.get(k, k) == k for _, ks in fixes for k in ks)

    # check the rules the Go engine uses (internal/pyre/charset.go) against the re module itself:
    # a cased literal c matches t when lower(t) is lower(c) or one of the fixes of lower(c); a class with a cased
    # member matches t when lower(t) is in the class lowered (plus fixes); anything else matches as is.
    fix_map = dict(fixes)

    def lit_matches(c, t):
        if not _sre.unicode_iscased(c):
            return t == c
        lo = _sre.unicode_tolower(c)
        return _sre.unicode_tolower(t) in (lo,) + fix_map.get(lo, ())

    samples = [*range(0x20, 0x7F), *range(0xA0, 0x250), 0x130, 0x131, 0x17F, 0x212A, 0x212B, 0x3A3, 0x3C2, 0x3C3,
               0x345, 0x3B9, 0x1FBE, 0x1E9E, 0xDF, 0x1E61, 0x1E9B, 0xFB05, 0xFB06, 0x10400, 0x13A0, 0xAB70]
    for cp in samples:
        want = matching(re.escape(chr(cp)), re.I)
        got = [t for t in CODEPOINTS if lit_matches(cp, t)]
        assert want == got, (hex(cp), want[:10], got[:10])

    buf = []
    w = buf.append
    w("// Code generated by tools/gen_tables.py from the Python %s standard library (Unicode %s). DO NOT EDIT.\n\n"
      % (sys.version.split()[0], unicodedata.unidata_version))
    w("package pyre\n\n")
    w("// unicodeVersion is the Unicode version of the tables (Python's unicodedata.unidata_version).\n")
    w("const unicodeVersion = %s\n\n" % go_str(unicodedata.unidata_version))
    w("// html5Entities is html.entities.html5: named character reference (with or without \";\"), NUL, replacement,\n")
    w("// NUL, ... in sorted order.\n")
    w("const html5Entities = \"\" +\n")
    for i in range(0, len(html5), 6):
        chunk = html5[i:i + 6]
        piece = "".join(k + "\x00" + v + "\x00" for k, v in chunk)
        w("\t%s%s\n" % (go_str(piece), " +" if i + 6 < len(html5) else ""))
    w("\n")
    int_slice(w, "invalidCharrefKeys", [k for k, _ in invalid_charrefs],
              "html._invalid_charrefs: numeric references that HTML maps to another character (keys)")
    str_slice(w, "invalidCharrefVals", [v for _, v in invalid_charrefs], "html._invalid_charrefs (values)")
    int_slice(w, "invalidCodepoints", invalid_codepoints, "html._invalid_codepoints: numeric references dropped")
    int_slice(w, "digitZeros", zeros, "the zero of every run of ten Unicode decimal digits (Python's \\d, int())")
    int_slice(w, "spaceRanges", flat(spaces), "Python's \\s and str.isspace(): inclusive ranges, first, last, ...")
    int_slice(w, "wordRanges", flat(words), "Python's \\w (str.isalnum() and \"_\"): inclusive ranges")
    int_slice(w, "isdigitRanges", flat(isdigit), "str.isdigit(): inclusive ranges")
    int_slice(w, "casedRanges", flat(cased), "str.lower(): characters that are cased and not case-ignorable (ranges)")
    int_slice(w, "caseIgnorableRanges", flat(ignorable), "str.lower(): case-ignorable characters (ranges)")
    lk = sorted(lower)
    int_slice(w, "lowerKeys", lk, "str.lower() of the non-ASCII characters it changes, keys (U+03A3 depends on the "
              "context)")
    str_slice(w, "lowerVals", [lower[k] for k in lk], "str.lower() (values)")
    uk = sorted(upper)
    int_slice(w, "upperKeys", uk, "str.upper() of the non-ASCII characters it changes, keys")
    str_slice(w, "upperVals", [upper[k] for k in uk], "str.upper() (values)")
    sk = sorted(sre_lower)
    int_slice(w, "sreLowerKeys", sk, "_sre.unicode_tolower: simple lowercase mapping used by re.IGNORECASE, keys")
    int_slice(w, "sreLowerVals", [sre_lower[k] for k in sk], "_sre.unicode_tolower (values)")
    int_slice(w, "sreCasedRanges", flat(sre_cased), "_sre.unicode_iscased: inclusive ranges")
    int_slice(w, "caseFixKeys", [k for k, _ in fixes], "re._casefix._EXTRA_CASES: lowercase character (keys)")
    str_slice(w, "caseFixVals", ["".join(map(chr, v)) for _, v in fixes],
              "re._casefix._EXTRA_CASES: the other characters that match it with re.IGNORECASE (values)")
    sys.stdout.write("".join(buf).rstrip("\n") + "\n")


if __name__ == "__main__":
    main()
