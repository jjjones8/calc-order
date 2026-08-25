package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var cellNamePattern = regexp.MustCompile(`^[A-Z]{1,3}[0-9]{1,7}$`)

// refPattern matches A1-style references, including the $A$1 absolute
// form. It over-matches slightly (it can't tell a reference from a bare
// word like "AND" followed by digits elsewhere in the string) but that's
// fine here since we only look inside formulas, which are mostly refs,
// numbers and operators.
var refPattern = regexp.MustCompile(`\$?[A-Za-z]{1,3}\$?[0-9]{1,7}`)

// rangePattern matches an A1:B10-style range, both ends in the same form
// refPattern accepts.
var rangePattern = regexp.MustCompile(`\$?[A-Za-z]{1,3}\$?[0-9]{1,7}:\$?[A-Za-z]{1,3}\$?[0-9]{1,7}`)

// refOrRangePattern tries the range form first: Go's regexp package
// resolves alternation left to right at a given position, so a range is
// preferred over reading its start cell as a lone reference.
var refOrRangePattern = regexp.MustCompile(rangePattern.String() + `|` + refPattern.String())

var cellRefSplitPattern = regexp.MustCompile(`^([A-Za-z]{1,3})([0-9]{1,7})$`)

// maxRangeCells bounds how many cells a single A1:B10-style range can
// expand to, so a typo like A1:A9999999 fails fast instead of allocating
// millions of cell names.
const maxRangeCells = 10000

// extractRefs returns the distinct cell references used by a formula, in
// the order they first appear, expanding any A1:B10-style ranges into
// their individual cells. Non-formula values (anything not starting with
// "=") have no references.
func extractRefs(formula string) ([]string, error) {
	if !strings.HasPrefix(formula, "=") {
		return nil, nil
	}
	formula = blankStringLiterals(formula)

	seen := make(map[string]bool)
	var refs []string
	add := func(ref string) {
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}

	for _, m := range refOrRangePattern.FindAllString(formula, -1) {
		if !strings.Contains(m, ":") {
			add(strings.ToUpper(strings.ReplaceAll(m, "$", "")))
			continue
		}
		expanded, err := expandRange(m)
		if err != nil {
			return nil, err
		}
		for _, ref := range expanded {
			add(ref)
		}
	}
	return refs, nil
}

// blankStringLiterals replaces the contents of any double-quoted string
// literal in formula with spaces, so text like ="A1 is the total" doesn't
// get read as a reference to cell A1. It keeps the string the same length
// so the result is still safe to feed to the ref-matching regexes. A
// doubled quote ("") inside a literal is the spreadsheet convention for an
// escaped quote character and does not end the literal.
func blankStringLiterals(formula string) string {
	b := []byte(formula)
	inString := false
	for i := 0; i < len(b); i++ {
		if b[i] != '"' {
			if inString {
				b[i] = ' '
			}
			continue
		}
		if inString && i+1 < len(b) && b[i+1] == '"' {
			b[i], b[i+1] = ' ', ' '
			i++
			continue
		}
		inString = !inString
		b[i] = ' '
	}
	return string(b)
}

// expandRange turns "A1:B10" (with optional $ signs) into every cell name
// in that rectangle, in row-major order.
func expandRange(rng string) ([]string, error) {
	parts := strings.SplitN(strings.ReplaceAll(rng, "$", ""), ":", 2)
	startCol, startRow, err := splitCellRef(parts[0])
	if err != nil {
		return nil, err
	}
	endCol, endRow, err := splitCellRef(parts[1])
	if err != nil {
		return nil, err
	}

	c1, c2 := colToNum(startCol), colToNum(endCol)
	if c1 > c2 {
		c1, c2 = c2, c1
	}
	r1, r2 := startRow, endRow
	if r1 > r2 {
		r1, r2 = r2, r1
	}
	if (c2-c1+1)*(r2-r1+1) > maxRangeCells {
		return nil, fmt.Errorf("range %s expands to more than %d cells", rng, maxRangeCells)
	}

	var cells []string
	for c := c1; c <= c2; c++ {
		col := numToCol(c)
		for r := r1; r <= r2; r++ {
			cells = append(cells, fmt.Sprintf("%s%d", col, r))
		}
	}
	return cells, nil
}

func splitCellRef(ref string) (col string, row int, err error) {
	m := cellRefSplitPattern.FindStringSubmatch(ref)
	if m == nil {
		return "", 0, fmt.Errorf("invalid cell reference %q", ref)
	}
	row, err = strconv.Atoi(m[2])
	if err != nil {
		return "", 0, err
	}
	return strings.ToUpper(m[1]), row, nil
}

// colToNum converts a column letter sequence (A, B, ..., Z, AA, AB, ...)
// to its 1-based number, matching spreadsheet column ordering.
func colToNum(letters string) int {
	n := 0
	for _, c := range strings.ToUpper(letters) {
		n = n*26 + int(c-'A'+1)
	}
	return n
}

// numToCol is the inverse of colToNum.
func numToCol(n int) string {
	var letters []byte
	for n > 0 {
		n--
		letters = append(letters, byte('A'+n%26))
		n /= 26
	}
	for i, j := 0, len(letters)-1; i < j; i, j = i+1, j-1 {
		letters[i], letters[j] = letters[j], letters[i]
	}
	return string(letters)
}

// topoSort orders the given cells so that every cell comes after the
// cells its own formula depends on. References to cells not present in
// the input are ignored, since they're presumably plain values supplied
// elsewhere. The result is deterministic: ties are broken alphabetically.
func topoSort(cells map[string]string) ([]string, error) {
	deps := make(map[string][]string)
	for cell, formula := range cells {
		refs, err := extractRefs(formula)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cell, err)
		}
		for _, ref := range refs {
			if ref == cell {
				return nil, fmt.Errorf("%s refers to itself", cell)
			}
			if _, ok := cells[ref]; ok {
				deps[cell] = append(deps[cell], ref)
			}
		}
	}
	for cell := range deps {
		sort.Strings(deps[cell])
	}

	names := make([]string, 0, len(cells))
	for c := range cells {
		names = append(names, c)
	}
	sort.Strings(names)

	const (
		unvisited = iota
		visiting
		done
	)
	state := make(map[string]int, len(cells))
	order := make([]string, 0, len(cells))
	var stack []string

	var visit func(string) error
	visit = func(cell string) error {
		switch state[cell] {
		case done:
			return nil
		case visiting:
			path := append(append([]string{}, stack...), cell)
			return fmt.Errorf("circular reference: %s", strings.Join(path, " -> "))
		}
		state[cell] = visiting
		stack = append(stack, cell)
		for _, dep := range deps[cell] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[cell] = done
		order = append(order, cell)
		return nil
	}

	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return order, nil
}
