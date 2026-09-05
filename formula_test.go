package main

import (
	"reflect"
	"testing"
)

func TestExtractRefs(t *testing.T) {
	cases := []struct {
		name         string
		formula      string
		defaultSheet string
		want         []string
		wantErr      bool
	}{
		{
			name:    "plain value has no refs",
			formula: "1200",
			want:    nil,
		},
		{
			name:    "empty string has no refs",
			formula: "",
			want:    nil,
		},
		{
			name:    "simple sum",
			formula: "=A1+A3",
			want:    []string{"A1", "A3"},
		},
		{
			name:    "duplicate refs collapse to first occurrence",
			formula: "=A1+A1+A2",
			want:    []string{"A1", "A2"},
		},
		{
			name:    "absolute refs are normalized",
			formula: "=$A$1+B$2+$C3",
			want:    []string{"A1", "B2", "C3"},
		},
		{
			name:    "lowercase refs are upper-cased",
			formula: "=a1+b2",
			want:    []string{"A1", "B2"},
		},
		{
			name:    "string literal refs are ignored",
			formula: `="Total: "&A1`,
			want:    []string{"A1"},
		},
		{
			name:    "doubled quote inside literal does not end it",
			formula: `="say ""A1"" here"&B2`,
			want:    []string{"B2"},
		},
		{
			name:    "range expands into individual cells",
			formula: "=SUM(A1:A3)",
			want:    []string{"A1", "A2", "A3"},
		},
		{
			name:    "range with reversed corners still expands in order",
			formula: "=SUM(B2:A1)",
			want:    []string{"A1", "A2", "B1", "B2"},
		},
		{
			name:    "range too large is an error",
			formula: "=SUM(A1:ZZ99999)",
			wantErr: true,
		},
		{
			name:    "sheet-qualified ref is normalized",
			formula: "=Sheet1!A1+sheet1!B2",
			want:    []string{"SHEET1!A1", "SHEET1!B2"},
		},
		{
			name:    "quoted sheet name with a space",
			formula: "='Sales Q1'!A1+A2",
			want:    []string{"SALES Q1!A1", "A2"},
		},
		{
			name:         "unqualified ref inherits the formula's own sheet",
			formula:      "=A1+Sheet2!B1",
			defaultSheet: "SHEET1",
			want:         []string{"SHEET1!A1", "SHEET2!B1"},
		},
		{
			name:    "sheet-qualified range expands with the sheet on every cell",
			formula: "=SUM(Sheet1!A1:B2)",
			want:    []string{"SHEET1!A1", "SHEET1!A2", "SHEET1!B1", "SHEET1!B2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractRefs(tc.formula, tc.defaultSheet)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("extractRefs(%q): expected error, got %v", tc.formula, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("extractRefs(%q): unexpected error: %v", tc.formula, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractRefs(%q) = %v, want %v", tc.formula, got, tc.want)
			}
		})
	}
}

func TestTopoSort(t *testing.T) {
	t.Run("orders cells after their dependencies", func(t *testing.T) {
		cells := map[string]string{
			"A1": "1200",
			"A2": "400",
			"A3": "150",
			"A4": "=A1+A3",
			"A5": "=A4+A2",
			"A6": "=A5*12",
		}
		got, err := topoSort(cells)
		if err != nil {
			t.Fatalf("topoSort: unexpected error: %v", err)
		}
		pos := make(map[string]int, len(got))
		for i, c := range got {
			pos[c] = i
		}
		if pos["A4"] < pos["A1"] || pos["A4"] < pos["A3"] {
			t.Errorf("A4 must come after A1 and A3, got order %v", got)
		}
		if pos["A5"] < pos["A4"] || pos["A5"] < pos["A2"] {
			t.Errorf("A5 must come after A4 and A2, got order %v", got)
		}
		if pos["A6"] < pos["A5"] {
			t.Errorf("A6 must come after A5, got order %v", got)
		}
	})

	t.Run("ties break alphabetically", func(t *testing.T) {
		cells := map[string]string{
			"B1": "1",
			"A1": "1",
			"C1": "1",
		}
		got, err := topoSort(cells)
		if err != nil {
			t.Fatalf("topoSort: unexpected error: %v", err)
		}
		want := []string{"A1", "B1", "C1"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("topoSort() = %v, want %v", got, want)
		}
	})

	t.Run("refs to cells outside the input are ignored", func(t *testing.T) {
		cells := map[string]string{
			"A1": "=Z9+1",
		}
		got, err := topoSort(cells)
		if err != nil {
			t.Fatalf("topoSort: unexpected error: %v", err)
		}
		want := []string{"A1"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("topoSort() = %v, want %v", got, want)
		}
	})

	t.Run("self reference is an error", func(t *testing.T) {
		cells := map[string]string{
			"A1": "=A1+1",
		}
		if _, err := topoSort(cells); err == nil {
			t.Fatal("topoSort: expected error for self reference, got nil")
		}
	})

	t.Run("direct cycle is reported", func(t *testing.T) {
		cells := map[string]string{
			"A": "=B",
			"B": "=A",
		}
		if _, err := topoSort(cells); err == nil {
			t.Fatal("topoSort: expected error for circular reference, got nil")
		}
	})

	t.Run("sheet-qualified ref depends on the other sheet's cell", func(t *testing.T) {
		cells := map[string]string{
			"SHEET1!A1": "10",
			"SHEET2!A1": "=Sheet1!A1*2",
		}
		got, err := topoSort(cells)
		if err != nil {
			t.Fatalf("topoSort: unexpected error: %v", err)
		}
		want := []string{"SHEET1!A1", "SHEET2!A1"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("topoSort() = %v, want %v", got, want)
		}
	})

	t.Run("unqualified ref on a sheet-qualified cell resolves to its own sheet", func(t *testing.T) {
		cells := map[string]string{
			"SHEET1!A1": "10",
			"SHEET1!A2": "=A1+1",
			"SHEET2!A1": "999",
		}
		got, err := topoSort(cells)
		if err != nil {
			t.Fatalf("topoSort: unexpected error: %v", err)
		}
		pos := make(map[string]int, len(got))
		for i, c := range got {
			pos[c] = i
		}
		if pos["SHEET1!A2"] < pos["SHEET1!A1"] {
			t.Errorf("SHEET1!A2 must come after SHEET1!A1, got order %v", got)
		}
	})

	t.Run("longer cycle is reported", func(t *testing.T) {
		cells := map[string]string{
			"A": "=B",
			"B": "=C",
			"C": "=A",
		}
		if _, err := topoSort(cells); err == nil {
			t.Fatal("topoSort: expected error for circular reference, got nil")
		}
	})
}

func TestNormalizeCellName(t *testing.T) {
	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{raw: "A1", want: "A1"},
		{raw: "a1", want: "A1"},
		{raw: "Sheet1!A1", want: "SHEET1!A1"},
		{raw: "sheet1!a1", want: "SHEET1!A1"},
		{raw: "'Sales Q1'!A1", want: "SALES Q1!A1"},
		{raw: "not a cell", wantErr: true},
		{raw: "Sheet1!!A1", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := normalizeCellName(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeCellName(%q): expected error, got %v", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeCellName(%q): unexpected error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("normalizeCellName(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
