# calcorder

When you pull formulas out of a spreadsheet (or hand-write a set of them
for some other tool to evaluate) you end up with a pile of cells that
reference each other in no particular order. Before anything can actually
calculate them, they need a sequence: every cell after the cells it
depends on. Get that wrong and you either compute stale values or walk
straight into a cycle.

calcorder answers exactly that question: given a set of `cell,formula`
pairs, what order can they be safely calculated in, and is there a
circular reference hiding in there?

It does not evaluate formulas or understand spreadsheet functions. It
only reads references out of them well enough to build a dependency
graph.

## Input format

One cell per line, either comma- or tab-separated, with a plain
spreadsheet-style cell reference (`A1`, `AA12`, ...) on the left:

```
<cell>,<formula>
```

A formula that doesn't start with `=` is treated as a plain value with no
dependencies. Blank lines and lines starting with `#` are ignored.

A formula can reference a range like `A1:B10`, which is expanded into its
individual cells (up to 10,000 of them; anything bigger is rejected as an
error rather than silently truncated).

```
A1,1200
A2,400
A3,150
A4,=A1+A3
A5,=A4+A2
A6,=A5*12
```

## Usage

From a file:

```
$ calcorder examples/budget.csv
A1
A2
A3
A4
A5
A6
```

From stdin, with no arguments:

```
$ cat examples/budget.csv | calcorder
```

Files and stdin can be mixed; use `-` to mean stdin explicitly:

```
$ cat extra.csv | calcorder examples/budget.csv -
```

When two inputs define the same cell, the later one wins.

If the formulas contain a cycle, calcorder reports it instead of
printing an order:

```
$ printf 'a,=b\nb,=a\n' | calcorder
calcorder: circular reference: a -> b -> a
```

Pass `-check` to skip printing the order entirely and just get an exit
code: 0 if the formulas are cycle-free, nonzero (with the cycle printed
to stderr) if not. Useful as a pre-commit or CI check on a formula file
without caring about the actual order.

```
$ calcorder -check examples/budget.csv; echo $?
0
```

## Build

```
go build -o calcorder .
```

## Limitations (for now)

- References inside string literals in a formula (e.g. `="A1"`) are
  matched anyway, since the parser doesn't understand quoting yet.
