#!/usr/bin/env python3
"""Generate internal/pyre/testdata/cases.jsonl.gz: Python's re results for the engine test (internal/pyre/pyre_test.go).

Each line has a pattern, the IGNORECASE flag, a text, and what Python returns for search, match at a position,
fullmatch, finditer (spans of every group) and sub with a template. The patterns are hand-picked corner cases plus
random patterns built from the constructs the parsing rules use (seeded, so the file is reproducible).

    python3.13 tools/gen_pyre_cases.py | gzip -n -9 > internal/pyre/testdata/cases.jsonl.gz
"""
import json
import random
import re
import sys
import warnings

warnings.simplefilter("ignore")

TEXTS = [
    "", "a", "ab", "aab", "abab", "ba", "a b", "a-b", "a  b", "ab\n", "a\nb", "\n", "12", "1 2", "x12y", "12ga",
    "9mm 15rd", "15+1", "10-22", "5.56x45", "g19 gen5", "Glock 19 Gen 5 MOS", "AbC aBc", "ſ s S", "ı i I İ",
    "K k K", "ß ss SS ẞ", "µ μ Μ", "ΑΣ", "١٥rd", "½ 1", "áb", "x‍y", "a_b", "a‿b", "\x1c12\x1f",
    "tab\tsep", "end.", ".308 win", "300 blk", "o/u 12/28", "20/25 box", "a,b,,c", "aaaa", "abcabc", "x" * 30,
]

FIXED = [
    r"a", r"a|b", r"ab|a", r"a*", r"a*?", r"a+", r"a+?", r"a?", r"a??", r"a{2}", r"a{2,}", r"a{,2}", r"a{1,2}?",
    r"a{", r"a{x}", r"a{}", r"{", r"x{,}", r"(a)|b", r"(a|ab)(c|bcd)?", r"(?:a|b)*", r"(a*)*", r"(a*)+", r"(a|b)*?b",
    r"(?:(a)|b)*", r"(?:x?)*y", r"(?:a?)+?b", r"(a?)*?b", r"((a)|b)+", r"(?:ab)*a", r"(?:a|)*b", r"(a|)+",
    r"^a", r"a$", r"^$", r"$", r"\Aa", r"a\Z", r"\b", r"\B", r"\ba\b", r"\Bb", r"\w+", r"\W+", r"\d+", r"\D",
    r"\s+", r"\S+", r"[a-c]+", r"[^a-c]+", r"[]a]", r"[^]a]", r"[a-]", r"[-a]", r"[\w-]+", r"[\d.]+", r"[\s\-]+",
    r"[.$]", r"[&~|]", r"[\]]", r"[\\]", r"[\x41-\x43]", r"\x41", r"é", r"\101", r"[\101]", r"\0", r"\.",
    r"\-", r"\ ", r"\#", r"\&", r"\~", r"(?<=a)b", r"(?<!a)b", r"(?<=ab)c?", r"(?<![a-z0-9.])\d+", r"\d+(?!\d)",
    r"\d+(?=rd)", r"(?=(a))a", r"(?!(a))\w", r"(?<=(a))b", r"(?=a|b)\w+", r"a(?=b)|ab", r"(?<=\d)\.(?=\d)",
    r"(?#comment)a", r"(?P<x>a)b", r"(?:(?:a)|(?:b))+", r"(a)(b)?(c)", r"((a)(b))", r"(\d{1,3})\s*\+\s*1(?!\d)",
    r"(\d{1,3}(?:,\d{3})+|\d{1,5})", r"(?:rd|rds|round)\b", r"\bgen\.?\s*-?\s*([1-6])\b", r"(?<![\d.:/x])(\d{1,3})",
    r"[^\t\n\f <&#;]{1,32};?", r"(sf|x|c|l|m|s)", r"[pue][a-z0-9]\d{3}", r"s", r"i", r"k", r"ß", r"[a-z]+",
    r"[^a-z0-9]+", r"[0-9a-z]", r"[A-Z]", r"[km]", r"[aä]", r"[^ä]", r"K", r"İ", r"ı", r"ſ",
    r"σ+", r"[Σ]", r".+", r".*", r"a.c", r"(.)x", r"(?:)", r"|", r"a|", r"|a",
    r"(?:\s*-?\s*g?\s*-?\s*|g)(\d{2})", r"(?:g([1-6])(?:us)?|(v))?", r"\s*-?\s*", r"[^,]{0,14}$", r",|\bor\b",
]


def rand_pattern(rng, depth=0):
    atoms = ["a", "b", "1", " ", "-", ".", r"\d", r"\w", r"\s", r"\b", "[ab]", "[^a]", "[a-c1]", "^", "$", "x"]
    parts = []
    for _ in range(rng.randint(1, 4)):
        r = rng.random()
        if depth < 2 and r < 0.18:
            inner = rand_pattern(rng, depth + 1)
            kind = rng.choice(["(", "(?:", "(?=", "(?!", "(?<=", "(?<!"])
            if kind.startswith("(?<"):
                inner = rng.choice(["a", "[ab]", r"\d", "ab", "a|b", r"\w\d"])
            parts.append(kind + inner + ")")
        elif depth < 2 and r < 0.28:
            parts.append("(?:" + rand_pattern(rng, depth + 1) + "|" + rand_pattern(rng, depth + 1) + ")")
        else:
            parts.append(rng.choice(atoms))
        if rng.random() < 0.35 and parts[-1] not in ("^", "$", r"\b"):
            q = rng.choice(["*", "+", "?", "{2}", "{1,2}", "{0,3}", "{2,}"])
            if rng.random() < 0.3:
                q += "?"
            parts[-1] = parts[-1] + q
    return "".join(parts)


def spans(m, n):
    return [list(m.span(g)) for g in range(n + 1)]


def case(pattern, flags, text, tmpl):
    rx = re.compile(pattern, flags)
    n = rx.groups
    out = {"p": pattern, "i": bool(flags & re.I), "t": text}
    m = rx.search(text)
    out["search"] = spans(m, n) if m else None
    pos = min(len(text), 1)
    m = rx.match(text, pos)
    out["pos"] = pos
    out["match"] = spans(m, n) if m else None
    m = rx.fullmatch(text)
    out["full"] = spans(m, n) if m else None
    out["iter"] = [spans(m, n) for m in rx.finditer(text)]
    out["tmpl"] = tmpl
    out["sub"] = rx.sub(tmpl, text)
    return out


def main():
    rng = random.Random(20261116)
    pats = list(FIXED)
    for _ in range(1500):
        pats.append(rand_pattern(rng))
    seen = set()
    w = sys.stdout.write
    for p in pats:
        for flags in (0, re.I):
            try:
                rx = re.compile(p, flags)
            except re.error:
                continue
            if (p, flags) in seen:
                continue
            seen.add((p, flags))
            tmpl = "<\\g<0>>" if rx.groups == 0 else "[\\1|\\g<%d>]" % rx.groups
            texts = TEXTS if p in FIXED else rng.sample(TEXTS, 6)
            for t in texts:
                w(json.dumps(case(p, flags, t, tmpl), ensure_ascii=True) + "\n")


if __name__ == "__main__":
    main()
