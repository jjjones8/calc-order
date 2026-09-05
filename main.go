// calcorder reads a list of spreadsheet cells and their formulas and prints
// the cells in an order safe to recalculate: every cell appears after the
// cells its formula depends on.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	checkOnly := flag.Bool("check", false, "only check for a circular reference; print nothing and exit nonzero if one is found")
	flag.Parse()

	cells, err := loadAll(flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, "calcorder:", err)
		os.Exit(1)
	}

	order, err := topoSort(cells)
	if err != nil {
		fmt.Fprintln(os.Stderr, "calcorder:", err)
		os.Exit(1)
	}

	if *checkOnly {
		return
	}

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for _, cell := range order {
		fmt.Fprintln(w, cell)
	}
}

// loadAll reads every argument as a file, falling back to stdin when no
// arguments are given. "-" means stdin explicitly, so stdin can be mixed
// in with real files on the same command line.
func loadAll(args []string) (map[string]string, error) {
	cells := make(map[string]string)
	if len(args) == 0 {
		args = []string{"-"}
	}

	for _, a := range args {
		if err := loadOne(cells, a); err != nil {
			return nil, fmt.Errorf("%s: %w", a, err)
		}
	}
	return cells, nil
}

func loadOne(cells map[string]string, path string) error {
	var r io.Reader
	if path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	}
	return loadInto(cells, r)
}

// loadInto parses "<cell>,<formula>" or "<cell><tab><formula>" lines.
// Blank lines and lines starting with # are ignored so files can carry
// comments and spacing without special-casing them elsewhere.
func loadInto(cells map[string]string, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		cell, formula, err := splitLine(text)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}
		cells[cell] = formula
	}
	return scanner.Err()
}

func splitLine(line string) (cell, formula string, err error) {
	sep := strings.IndexAny(line, ",\t")
	if sep < 0 {
		return "", "", fmt.Errorf("expected <cell><comma or tab><formula>, got %q", line)
	}
	cell, err = normalizeCellName(strings.TrimSpace(line[:sep]))
	if err != nil {
		return "", "", err
	}
	formula = strings.TrimSpace(line[sep+1:])
	return cell, formula, nil
}
