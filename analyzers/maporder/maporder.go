// Package maporder reports order-sensitive assertions on data accumulated by
// ranging over a map.
//
// Go deliberately randomizes map iteration order on every run, so a slice or
// string built by ranging over a map has no stable order. Asserting it against
// a fixed expected value with an order-sensitive check (assert.Equal,
// reflect.DeepEqual, slices.Equal, ...) passes only when the random order
// happens to match — the test can go green for months and then flake. Sorting
// the accumulator first, or using an order-insensitive assertion such as
// assert.ElementsMatch, removes the dependency.
//
// The check fires only when the whole story is visible in one test unit: a
// map-range loop appends into a local accumulator, and that accumulator later
// reaches an order-sensitive assertion without being sorted, escaping the unit,
// or being compared order-insensitively. Any use it cannot classify is treated
// as an escape and silences the finding. testify calls are matched only in
// their package-level form (assert.Equal(t, ...)), not the require.New(t)
// method form.
package maporder

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/malikov73/flakylint/internal/testfuncs"
)

var Analyzer = &analysis.Analyzer{
	Name:     "maporder",
	Doc:      "reports order-sensitive assertions on values accumulated from a map range; Go randomizes map iteration order, so such tests flake",
	URL:      "https://github.com/malikov73/flakylint",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

const msg = "assertion depends on map iteration order, which Go randomizes on every run — sort the values first or use an order-insensitive assertion such as assert.ElementsMatch"

type pkgFunc struct{ pkg, name string }

var testifyPkgs = []string{
	"github.com/stretchr/testify/assert",
	"github.com/stretchr/testify/require",
}

// sensitiveTestify are the order-sensitive equality assertions; sensitivePkg
// are their stdlib counterparts.
var (
	sensitiveTestify = []string{"Equal", "Equalf", "EqualValues", "EqualValuesf"}
	sensitivePkg     = []pkgFunc{{"reflect", "DeepEqual"}, {"slices", "Equal"}, {"slices", "EqualFunc"}}
)

// insensitiveTestify are assertions that ignore order (or only look at length
// or membership); their presence on an accumulator means the test does not
// depend on iteration order.
var insensitiveTestify = []string{
	"ElementsMatch", "ElementsMatchf",
	"Len", "Lenf",
	"Contains", "Containsf",
	"Subset", "Subsetf",
	"SameElements",
}

// sortPkg are calls that reorder the accumulator in place, giving it a stable
// order before any assertion.
var sortPkg = []pkgFunc{
	{"sort", "Strings"}, {"sort", "Ints"}, {"sort", "Float64s"},
	{"sort", "Slice"}, {"sort", "SliceStable"}, {"sort", "Sort"},
	{"slices", "Sort"}, {"slices", "SortFunc"}, {"slices", "SortStableFunc"},
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil), (*ast.CallExpr)(nil)}, func(n ast.Node) {
		if !testfuncs.InTestFile(pass, n) {
			return
		}
		var body *ast.BlockStmt
		switch n := n.(type) {
		case *ast.FuncDecl:
			if _, ok := testfuncs.TestFunc(pass.TypesInfo, n); !ok {
				return
			}
			body = n.Body
		case *ast.CallExpr:
			lit, _, ok := testfuncs.SubtestLit(pass.TypesInfo, n)
			if !ok {
				return
			}
			body = lit.Body
		}
		checkUnit(pass, body)
	})
	return nil, nil
}

// checkUnit analyzes one test unit (a test function body or a subtest literal
// body). Nested function literals — goroutines, callbacks, and nested subtest
// literals — are excluded from accumulator detection and count as escapes when
// they capture an accumulator; each nested subtest is analyzed as its own unit.
func checkUnit(pass *analysis.Pass, body *ast.BlockStmt) {
	info := pass.TypesInfo

	fromMap, outside, writeLHS := collectAccumulators(info, body)
	declared := declaredObjects(info, body)

	candidate := map[types.Object]bool{}
	for obj := range fromMap {
		if !outside[obj] && declared[obj] {
			candidate[obj] = true
		}
	}
	if len(candidate) == 0 {
		return
	}

	benign := writeLHS // accumulator write targets are part of the accumulation
	sorted := map[types.Object]bool{}
	insensitive := map[types.Object]bool{}
	escaped := map[types.Object]bool{}
	sensitive := map[types.Object]*ast.CallExpr{}

	// Pass 1: classify every call, and treat any capture of an accumulator by a
	// nested function literal as an escape.
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			ast.Inspect(node, func(m ast.Node) bool {
				if id, ok := m.(*ast.Ident); ok && candidate[info.Uses[id]] {
					escaped[info.Uses[id]] = true
				}
				return true
			})
			return false
		case *ast.CallExpr:
			kind := classify(info, node)
			for _, arg := range node.Args {
				id, ok := arg.(*ast.Ident)
				if !ok {
					continue
				}
				obj := info.Uses[id]
				if !candidate[obj] {
					continue
				}
				switch kind {
				case kindSort:
					sorted[obj] = true
					benign[id] = true
				case kindInsensitive:
					insensitive[obj] = true
					benign[id] = true
				case kindSensitive:
					if sensitive[obj] == nil {
						sensitive[obj] = node
					}
					benign[id] = true
				case kindBuiltin:
					benign[id] = true
				case kindOther:
					escaped[obj] = true
				}
			}
		}
		return true
	})

	// Pass 2: any remaining use of an accumulator (return, assignment, address,
	// index, selector, ...) that was not classified above is an escape.
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false // handled in pass 1
		}
		id, ok := n.(*ast.Ident)
		if !ok || benign[id] {
			return true
		}
		if obj := info.Uses[id]; candidate[obj] {
			escaped[obj] = true
		}
		return true
	})

	for obj, call := range sensitive {
		if sorted[obj] || insensitive[obj] || escaped[obj] {
			continue
		}
		pass.Report(analysis.Diagnostic{Pos: call.Pos(), End: call.End(), Message: msg})
	}
}

type callKind int

const (
	kindOther callKind = iota
	kindBuiltin
	kindSort
	kindInsensitive
	kindSensitive
)

func classify(info *types.Info, call *ast.CallExpr) callKind {
	if id, ok := call.Fun.(*ast.Ident); ok {
		if b, ok := info.Uses[id].(*types.Builtin); ok {
			switch b.Name() {
			case "append", "len", "cap":
				return kindBuiltin
			}
			return kindOther
		}
	}
	for _, pf := range sortPkg {
		if testfuncs.IsPkgFunc(info, call, pf.pkg, pf.name) {
			return kindSort
		}
	}
	for _, pf := range sensitivePkg {
		if testfuncs.IsPkgFunc(info, call, pf.pkg, pf.name) {
			return kindSensitive
		}
	}
	for _, p := range testifyPkgs {
		for _, name := range sensitiveTestify {
			if testfuncs.IsPkgFunc(info, call, p, name) {
				return kindSensitive
			}
		}
		for _, name := range insensitiveTestify {
			if testfuncs.IsPkgFunc(info, call, p, name) {
				return kindInsensitive
			}
		}
	}
	return kindOther
}

// collectAccumulators finds the accumulators written inside map-range loops
// (fromMap), those also written outside any map-range loop (outside — mixed
// provenance), and the set of accumulator-write target identifiers (writeLHS),
// which belong to the accumulation and must not be read as escapes.
func collectAccumulators(info *types.Info, body *ast.BlockStmt) (fromMap, outside map[types.Object]bool, writeLHS map[*ast.Ident]bool) {
	fromMap = map[types.Object]bool{}
	outside = map[types.Object]bool{}
	writeLHS = map[*ast.Ident]bool{}

	inMapWrite := map[*ast.AssignStmt]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		rs, ok := n.(*ast.RangeStmt)
		if !ok || !isMapRange(info, rs) {
			return true
		}
		eachAccWrite(info, rs.Body, func(obj types.Object, assign *ast.AssignStmt, _ *ast.Ident) {
			fromMap[obj] = true
			inMapWrite[assign] = true
		})
		return true
	})

	eachAccWrite(info, body, func(obj types.Object, assign *ast.AssignStmt, id *ast.Ident) {
		writeLHS[id] = true
		if !inMapWrite[assign] {
			outside[obj] = true
		}
	})
	return fromMap, outside, writeLHS
}

// eachAccWrite calls fn for every accumulator write directly inside node
// (skipping nested function literals): a slice append `acc = append(acc, ...)`
// or a string concatenation `acc += ...`.
func eachAccWrite(info *types.Info, node ast.Node, fn func(obj types.Object, assign *ast.AssignStmt, id *ast.Ident)) {
	ast.Inspect(node, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		id, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		obj := info.Uses[id]
		if obj == nil {
			return true
		}
		switch assign.Tok {
		case token.ASSIGN:
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok || !isAppendOf(info, call, obj) {
				return true
			}
		case token.ADD_ASSIGN:
			if !isString(obj.Type()) {
				return true // numeric += sums commutatively: order-independent
			}
		default:
			return true
		}
		fn(obj, assign, id)
		return true
	})
}

// isAppendOf reports whether call is append(obj, ...).
func isAppendOf(info *types.Info, call *ast.CallExpr, obj types.Object) bool {
	id, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	if b, ok := info.Uses[id].(*types.Builtin); !ok || b.Name() != "append" {
		return false
	}
	if len(call.Args) == 0 {
		return false
	}
	a0, ok := call.Args[0].(*ast.Ident)
	return ok && info.Uses[a0] == obj
}

// declaredObjects returns the variables declared directly in body (outside
// nested function literals), used to require an accumulator to be local to the
// unit.
func declaredObjects(info *types.Info, body *ast.BlockStmt) map[types.Object]bool {
	declared := map[types.Object]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if id, ok := n.(*ast.Ident); ok {
			if obj := info.Defs[id]; obj != nil {
				declared[obj] = true
			}
		}
		return true
	})
	return declared
}

func isMapRange(info *types.Info, rs *ast.RangeStmt) bool {
	t := info.TypeOf(rs.X)
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Map)
	return ok
}

func isString(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}
