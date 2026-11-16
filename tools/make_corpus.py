#!/usr/bin/env python3
"""Build a crosscheck corpus from the Python package (the reference implementation): inputs plus the output of every
public function, for TestCrosscheck in crosscheck_test.go.

    python3.13 tools/make_corpus.py OUT.jsonl [--python ../library/python/src] [--golden testdata/golden.jsonl]
                                    [--catalog ../library/testdata] [--datasets ../datasets]

Inputs:
* every golden title with and without brand/mpn, and with adversarial edits (case, NBSP, en dash, HTML-escaped,
  smart quotes, accents, combining marks, zero-width joiner, Arabic-Indic digits, astral letters, emoji, dotless i,
  long s, Kelvin sign, sharp s, broken entities);
* with --catalog: the GunsNPrices catalog titles (lib_guns.tsv with manufacturer, mpn and sku; lib_ammo.tsv with the
  price);
* with --datasets: caliber and model names from the open datasets.

Then: GUNSNPRICES_CORPUS=OUT.jsonl go test -run Crosscheck -v

Without a local copy of the Python sources, install the package (pip install gunsnprices==0.1.2): the import falls
back to it.
"""
import argparse
import csv
import html
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ap = argparse.ArgumentParser()
ap.add_argument("out")
ap.add_argument("--python", default=os.path.join(HERE, "..", "..", "library", "python", "src"))
ap.add_argument("--golden", default=os.path.join(HERE, "..", "testdata", "golden.jsonl"))
ap.add_argument("--catalog", default=None)
ap.add_argument("--datasets", default=None)
opt = ap.parse_args()
sys.path.insert(0, opt.python)
import gunsnprices as g  # noqa: E402

items = []  # (title, brand, mpn, sku, price)


def add(t, brand=None, mpn="", sku="", price=None):
    if t is not None:
        items.append((t, brand, mpn, sku, price))


for line in open(opt.golden, encoding="utf-8"):
    if not line.strip():
        continue
    a = json.loads(line)["args"]
    t = a["title"]
    add(t, a.get("brand"), a.get("mpn", ""), a.get("sku", ""), a.get("price"))
    add(t)                                                  # without brand/mpn
    add(t.upper())
    add(t.lower())
    add(t.replace(" ", " "))                            # NBSP
    add(t.replace(" ", "  ").replace("-", "–"))         # double spaces, en dash
    add(html.escape(t))                                     # &quot; &amp; ...
    add(t.replace('"', "”").replace("'", "’"))    # smart quotes
    add(t + " ½")                                       # vulgar fraction (Python \w: yes)
    add("Édition " + t.replace("e", "é", 2))       # accents
    add(t.replace("e", "é", 1))                        # combining accent (Python \w: no)
    add(t.replace(" ", "‿ ", 1))                        # connector punctuation (Python \w: no)
    add(t.replace(" ", "‍ ", 1))                        # zero width joiner
    add(t.replace("1", "١", 1))                         # Arabic-Indic digit
    add(t.replace("9", "９", 1))                         # fullwidth digit
    add("\U0001d504 " + t)                                   # astral letter (math fraktur A)
    add(t + " \U0001f52b")                                   # emoji
    add(t.replace(" ", " | ", 1).replace(" ", " * ", 1) + "®™")
    add(t.replace("i", "ı", 1))                         # dotless i (Python re.I: matches i)
    add(t.replace("I", "İ", 1))                         # dotted capital I
    add(t.replace("s", "ſ", 1))                         # long s (Python re.I: matches s)
    add(t.replace("k", "K", 1))                         # Kelvin sign (Python re.I: matches k)
    add(t.replace("ss", "ß", 1).replace("SS", "ẞ", 1))  # sharp s (Python re.I: does not match "ss")
    add(t.replace("A", "Α", 1).replace("E", "Σ", 1))    # Greek capitals, final sigma in lower()
    add(t.replace("&quot;", "&#34;").replace("&amp;", "&amp"))
    if a.get("price"):
        add(t, None, "", "", a["price"] * 3.7)

if opt.catalog:
    for row in csv.DictReader(open(os.path.join(opt.catalog, "lib_guns.tsv"), encoding="utf-8"), delimiter="\t"):
        if row["name"]:
            add(row["name"], row.get("manufacturer") or None, row.get("mpn") or "", row.get("sku") or "")
    for row in csv.DictReader(open(os.path.join(opt.catalog, "lib_ammo.tsv"), encoding="utf-8"), delimiter="\t"):
        if row["name"]:
            p = row.get("instock_min")
            add(row["name"], None, "", "", float(p) if p not in (None, "", "0.00") else None)

D = opt.datasets
if D:
    for f, col in (("src/bg_caliber_freq.tsv", 0), ("src/bg_caliber_map.tsv", 0), ("src/bg_caliber_map.tsv", 1),
                   ("src/gnp_calibers.tsv", 0), ("src/site_l2.tsv", 1), ("src/site_cats.tsv", 2),
                   ("src/models_m2.tsv", 3), ("src/models_m3.tsv", 2)):
        path = os.path.join(D, f)
        if os.path.exists(path):
            for row in csv.reader(open(path, encoding="utf-8"), delimiter="\t"):
                if len(row) > col:
                    add(row[col])
    path = os.path.join(D, "firearm-models", "models.csv")
    if os.path.exists(path):
        for row in csv.DictReader(open(path, encoding="utf-8")):
            add(row["model"])
            add(row["model"], row["brand"])
            add(row["family"] + " " + row["calibers"].split(" | ")[0])
            add(row["model"] + " " + row["calibers"])
    path = os.path.join(D, "firearm-calibers", "calibers.csv")
    if os.path.exists(path):
        for row in csv.DictReader(open(path, encoding="utf-8")):
            for k in row:
                add(row[k])

# tricky entities and regex corner cases
for t in ["&amp;quot;", "&#0;", "&#x110000;", "&#99999999999999999999;", "&ampfoo", "&notit;", "&#x80;", "&#128;",
          "&#xD800;", "&AElig", "&Afr;", "&#65", "&#x41", "a&b", "&", "&;", "&#;", "&#x;", "x &frac12; y",
          "&NotNestedGreaterGreater;", "ΑΣ", "ΑΣ́ x", "line\nbreak 9mm", "9mm\n", "",
          "\x1c12ga\x1f", "glock 19\u0085gen 5", "1/2 in barrel"]:
    add(t)
    add("Glock 19 " + t + " 9mm 15rd")

seen = set()
n = 0
with open(opt.out, "w", encoding="utf-8") as out:
    for item in items:
        if item in seen:
            continue
        seen.add(item)
        t, brand, mpn, sku, price = item
        info = g.parse_title(t, brand=brand, mpn=mpn, sku=sku, price=price).to_dict()
        m = g.identify_model(t, brand=brand, mpn=mpn, sku=sku)
        nc = g.normalize_caliber(t)
        rec = {"in": {"title": t, "brand": brand, "mpn": mpn, "sku": sku, "price": price},
               "out": {"parse": info,
                       "model": None if m is None else {"brand": m.brand, "family": m.family, "model": m.model,
                                                         "generation": m.generation, "configs": m.configs,
                                                         "features": m.features, "source": m.source},
                       "find": [c.id for c in g.find_calibers(t)], "norm": nc.id if nc else None,
                       "cap": g.magazine_capacity(t), "bbl": g.barrel_length(t), "rpb": g.rounds_per_box(t),
                       "ammo": g.looks_like_ammo(t), "unescape": html.unescape(t)}}
        out.write(json.dumps(rec, ensure_ascii=True) + "\n")
        n += 1
print("records", n, file=sys.stderr)
