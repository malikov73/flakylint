// Package nolint makes analyzers honor //nolint suppression comments.
//
// A diagnostic on line L is dropped when a //nolint or
// //nolint:<name>[,<name>...] comment sits on line L or on line L-1.
// A bare //nolint suppresses every check; a named directive suppresses
// only the checks it lists. A trailing explanation is allowed, e.g.
// //nolint:sleepassert // inherited test.
package nolint

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Wrap returns a copy of a whose diagnostics are dropped when a
// //nolint or //nolint:<name>[,<name>...] comment is present on the
// reported line or on the line directly above it.
func Wrap(a *analysis.Analyzer) *analysis.Analyzer {
	inner := a.Run
	wrapped := *a
	wrapped.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		pass.Report = func(d analysis.Diagnostic) {
			if suppressed(pass, wrapped.Name, d.Pos) {
				return
			}
			report(d)
		}
		return inner(pass)
	}
	return &wrapped
}

// suppressed reports whether a //nolint directive covering check name sits
// on the line of pos or the line directly above it.
func suppressed(pass *analysis.Pass, name string, pos token.Pos) bool {
	tf := pass.Fset.File(pos)
	if tf == nil {
		return false
	}
	var file *ast.File
	for _, f := range pass.Files {
		if pass.Fset.File(f.Pos()) == tf {
			file = f
			break
		}
	}
	if file == nil {
		return false
	}
	line := tf.Line(pos)
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			cl := tf.Line(c.Pos())
			if cl != line && cl != line-1 {
				continue
			}
			names, ok := parseNolint(c.Text)
			if !ok {
				continue
			}
			if len(names) == 0 {
				return true // bare //nolint suppresses everything
			}
			for _, n := range names {
				if n == name {
					return true
				}
			}
		}
	}
	return false
}

// parseNolint parses a //nolint directive. It returns the checks named by
// the directive (nil for a bare //nolint) and whether text is a directive at
// all. //nolintfoo and prose are not directives.
func parseNolint(text string) (names []string, ok bool) {
	rest, found := strings.CutPrefix(text, "//nolint")
	if !found {
		return nil, false
	}
	if rest == "" || rest[0] == ' ' || rest[0] == '\t' {
		return nil, true // bare //nolint, optionally with a trailing reason
	}
	spec, found := strings.CutPrefix(rest, ":")
	if !found {
		return nil, false // //nolintfoo, not a directive
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if i := strings.IndexAny(part, " \t"); i >= 0 {
			part = part[:i] // drop a trailing "// reason" tail
		}
		if part != "" {
			names = append(names, part)
		}
	}
	return names, true
}
