package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var cellNamePattern = regexp.MustCompile(`^[A-Z]{1,3}[0-9]{1,7}$`)

// refPattern matches A1-style references, including the $A$1 absolute
// form. It over-matches slightly (it can't tell a reference from a bare
// word like "AND" followed by digits elsewhere in the string) but that's
// fine here since we only look inside formulas, which are mostly refs,
// numbers and operators.
var refPattern = regexp.MustCompile(`\$?[A-Za-z]{1,3}\$?[0-9]{1,7}`)

// extractRefs returns the distinct cell references used by a formula, in
// the order they first appear. Non-formula values (anything not starting
// with "=") have no references.
func extractRefs(formula string) []string {
	if !strings.HasPrefix(formula, "=") {
		return nil
	}

	seen := make(map[string]bool)
	var refs []string
	for _, m := range refPattern.FindAllString(formula, -1) {
		ref := strings.ToUpper(strings.ReplaceAll(m, "$", ""))
		if seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	return refs
}

// topoSort orders the given cells so that every cell comes after the
// cells its own formula depends on. References to cells not present in
// the input are ignored, since they're presumably plain values supplied
// elsewhere. The result is deterministic: ties are broken alphabetically.
func topoSort(cells map[string]string) ([]string, error) {
	deps := make(map[string][]string)
	for cell, formula := range cells {
		for _, ref := range extractRefs(formula) {
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
