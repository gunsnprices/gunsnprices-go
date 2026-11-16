// Command gunsnprices parses firearm and ammunition product titles and prints the result as JSON.
//
// With titles as arguments it prints one indented JSON object for the title (the arguments joined with spaces).
// Without arguments it reads titles from standard input, one per line, and writes JSON Lines.
//
//	gunsnprices "Ruger GP100 .357 Mag / .38 Special 4.2in 6-shot Revolver"
//	gunsnprices -price 18.99 "Federal 9mm 115gr FMJ 50/Box"
//	gunsnprices < titles.txt > parsed.jsonl
//
// Install with: go install github.com/gunsnprices/gunsnprices-go/cmd/gunsnprices@latest
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gunsnprices/gunsnprices-go"
	"github.com/gunsnprices/gunsnprices-go/internal/pyre"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gunsnprices", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: gunsnprices [flags] [title ...]")
		fmt.Fprintln(stderr, "Parse firearm and ammunition product titles (reads standard input when no title is given).")
		fs.PrintDefaults()
	}
	brand := fs.String("brand", "", "manufacturer, if known")
	mpn := fs.String("mpn", "", "manufacturer part number, if known")
	sku := fs.String("sku", "", "store SKU, if known")
	price := fs.Float64("price", 0, "listing price (gives cost per round for ammo)")
	kind := fs.String("kind", "", `"firearm" or "ammo" (skips the guess)`)
	version := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *version {
		fmt.Fprintln(stdout, "gunsnprices", gunsnprices.Version)
		return 0
	}
	if *kind != "" && *kind != string(gunsnprices.Firearm) && *kind != string(gunsnprices.Ammo) {
		fmt.Fprintf(stderr, "gunsnprices: -kind must be firearm or ammo, not %q\n", *kind)
		return 2
	}
	opts := &gunsnprices.Options{Brand: *brand, MPN: *mpn, SKU: *sku, Price: *price, Kind: gunsnprices.Kind(*kind)}
	out := bufio.NewWriter(stdout)
	defer out.Flush()
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	if fs.NArg() > 0 {
		enc.SetIndent("", "  ")
		if err := enc.Encode(gunsnprices.ParseTitle(strings.Join(fs.Args(), " "), opts)); err != nil {
			fmt.Fprintln(stderr, "gunsnprices:", err)
			return 1
		}
		return 0
	}
	in := bufio.NewReader(stdin)
	for {
		line, err := in.ReadString('\n')
		if t := pyre.Strip(line); t != "" { // Python: line.strip()
			if err := enc.Encode(gunsnprices.ParseTitle(t, opts)); err != nil {
				fmt.Fprintln(stderr, "gunsnprices:", err)
				return 1
			}
		}
		if err == io.EOF {
			return 0
		}
		if err != nil {
			fmt.Fprintln(stderr, "gunsnprices:", err)
			return 1
		}
	}
}
