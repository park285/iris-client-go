package webhook

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	constantTimeHelperName = "constantTimeEqualString"
	subtleImportPath       = "crypto/subtle"
)

// 식별자를 camelCase·snake_case 단어로 나눈다(bodySHA256 → body, SHA256).
var identifierWordPattern = regexp.MustCompile(`[A-Z]+\d*(?:[a-z]+\d*)?|[a-z]+\d*|\d+`)

// 식별자의 어느 단어라도 다음 중 하나이면 타이밍 공격 대상인 비밀 값으로 본다(sigHex, tokenBytes).
var secretWords = []string{"signature", "sig", "token", "secret", "digest", "sha256", "mac", "hmac"}

// 마지막 단어가 다음 중 하나이면 비밀 값에서 파생된 비비밀 메타데이터로 본다(tokenErr, hmacImportPath, jsonToken.Kind).
var nonSecretSuffixes = []string{"err", "error", "path", "header", "len", "count", "kind"}

// 비밀 값은 subtle.ConstantTimeCompare(constantTimeEqualString)로만 비교한다.
// 빈 값·nil 같은 리터럴이나 이름 있는 상수와의 비교는 존재 확인·열거 판정이라 허용한다. 단 리터럴의 반대쪽이 호출이면
// strings.Compare(sig, want) != 0처럼 비교 결과를 리터럴과 맞추는 우회일 수 있어 허용하지 않는다.
func TestSecretValuesAreNotComparedWithEqualityOperators(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()

	var files []*ast.File

	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return skipNonSourceDir(entry.Name())
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}

		files = append(files, file)

		return nil
	})
	if err != nil {
		t.Fatalf("scan module sources: %v", err)
	}

	constants := constantNames(files)
	for _, file := range files {
		for _, position := range secretEqualityComparisons(file, constants) {
			t.Errorf("%s: compare secret values with %s, not ==/!=/switch", fileSet.Position(position), constantTimeHelperName)
		}
	}
}

// 위 검사가 허용하는 유일한 비교 경로인 helper 자체가 상수 시간 비교여야 한다.
// 인자 이름이 left/right라 이름 휴리스틱으로는 보호되지 않으므로 본문 구조를 직접 단언한다.
func TestConstantTimeHelperUsesSubtleCompare(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	file, helper := findConstantTimeHelper(t, fileSet)

	subtleName, imported := importName(file, subtleImportPath)
	if !imported {
		t.Fatalf("%s must import %s", fileSet.Position(file.Package), subtleImportPath)
	}

	for _, problem := range constantTimeHelperProblems(helper, subtleName) {
		t.Errorf("%s: %s: %s", fileSet.Position(problem.pos), constantTimeHelperName, problem.message)
	}
}

// scripts/는 저장소 도구라 런타임 비밀 값을 다루지 않는다.
func skipNonSourceDir(name string) error {
	if name == "testdata" || name == "vendor" || name == "scripts" || strings.HasPrefix(name, ".") && name != ".." {
		return filepath.SkipDir
	}

	return nil
}

func findConstantTimeHelper(t *testing.T, fileSet *token.FileSet) (*ast.File, *ast.FuncDecl) {
	t.Helper()

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("list webhook sources: %v", err)
	}

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		file, parseErr := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		for _, decl := range file.Decls {
			if helper, ok := decl.(*ast.FuncDecl); ok && helper.Recv == nil && helper.Name.Name == constantTimeHelperName {
				return file, helper
			}
		}
	}

	t.Fatalf("webhook package must define %s", constantTimeHelperName)

	return nil, nil
}

func importName(file *ast.File, importPath string) (string, bool) {
	for _, spec := range file.Imports {
		if path, err := strconv.Unquote(spec.Path.Value); err != nil || path != importPath {
			continue
		}

		if spec.Name != nil {
			return spec.Name.Name, true
		}

		return filepath.Base(importPath), true
	}

	return "", false
}

type helperProblem struct {
	pos     token.Pos
	message string
}

// helper 본문은 두 매개변수를 각각 인자로 받는 subtle.ConstantTimeCompare 호출과 []byte 변환만 쓰고,
// ==/!=는 그 호출 결과를 1(같음) 또는 0(다름)과 맞추는 데만 쓸 수 있다.
func constantTimeHelperProblems(helper *ast.FuncDecl, subtleName string) []helperProblem {
	var (
		problems     []helperProblem
		compareCalls int
	)

	params := parameterNames(helper)

	ast.Inspect(helper.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			switch {
			case isSubtleCompareCall(value, subtleName):
				compareCalls++

				if !comparesBothParameters(value, params) {
					problems = append(problems, helperProblem{value.Pos(), "ConstantTimeCompare must compare the two parameters"})
				}
			case !isByteSliceConversion(value):
				problems = append(problems, helperProblem{value.Pos(), "only subtle.ConstantTimeCompare and []byte conversions are allowed"})
			}
		case *ast.BinaryExpr:
			if (value.Op == token.EQL || value.Op == token.NEQ) && !isSubtleResultCheck(value, subtleName) {
				problems = append(problems, helperProblem{value.Pos(), "==/!= is allowed only on the ConstantTimeCompare result"})
			}
		case *ast.SwitchStmt:
			problems = append(problems, helperProblem{value.Pos(), "switch comparisons are not constant-time"})
		}

		return true
	})

	if compareCalls == 0 {
		problems = append(problems, helperProblem{helper.Pos(), "must call " + subtleName + ".ConstantTimeCompare"})
	}

	return problems
}

func parameterNames(helper *ast.FuncDecl) []string {
	var names []string

	for _, field := range helper.Type.Params.List {
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}

	return names
}

func isSubtleCompareCall(call *ast.CallExpr, subtleName string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "ConstantTimeCompare" {
		return false
	}

	pkg, ok := selector.X.(*ast.Ident)

	return ok && pkg.Name == subtleName
}

func isByteSliceConversion(call *ast.CallExpr) bool {
	array, ok := call.Fun.(*ast.ArrayType)
	if !ok || array.Len != nil {
		return false
	}

	elem, ok := array.Elt.(*ast.Ident)

	return ok && elem.Name == "byte"
}

func comparesBothParameters(call *ast.CallExpr, params []string) bool {
	if len(call.Args) != 2 || len(params) != 2 {
		return false
	}

	first, second := referencedNames(call.Args[0]), referencedNames(call.Args[1])

	return first[params[0]] && second[params[1]] || first[params[1]] && second[params[0]]
}

func referencedNames(expr ast.Expr) map[string]bool {
	names := map[string]bool{}

	ast.Inspect(expr, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok {
			names[ident.Name] = true
		}

		return true
	})

	return names
}

func isSubtleResultCheck(binary *ast.BinaryExpr, subtleName string) bool {
	want := "1"

	if binary.Op == token.NEQ {
		want = "0"
	}

	for _, pair := range [][2]ast.Expr{{binary.X, binary.Y}, {binary.Y, binary.X}} {
		call, isCall := ast.Unparen(pair[0]).(*ast.CallExpr)
		literal, isLiteral := ast.Unparen(pair[1]).(*ast.BasicLit)

		if isCall && isLiteral && isSubtleCompareCall(call, subtleName) && literal.Value == want {
			return true
		}
	}

	return false
}

// secretScanner는 선언 하나 안에서 비밀 값을 복사·변환해 받은 지역 이름(want, got := expected, signature)을 추적한다.
// 이름 있는 상수와 import한 패키지 이름(sha256.Sum256의 sha256)은 공개 값이라 비밀 값으로 보지 않는다.
type secretScanner struct {
	constants map[string]bool
	packages  map[string]bool
	tainted   map[string]bool
}

func secretEqualityComparisons(file *ast.File, constants map[string]bool) []token.Pos {
	positions := make([]token.Pos, 0, len(file.Decls))

	packages := map[string]bool{}

	for _, spec := range file.Imports {
		if path, err := strconv.Unquote(spec.Path.Value); err == nil {
			packages[filepath.Base(path)] = true
		}

		if spec.Name != nil {
			packages[spec.Name.Name] = true
		}
	}

	for _, decl := range file.Decls {
		scanner := secretScanner{constants: constants, packages: packages, tainted: map[string]bool{}}
		scanner.propagate(decl)

		positions = append(positions, scanner.comparisons(decl)...)
	}

	return positions
}

func constantNames(files []*ast.File) map[string]bool {
	names := map[string]bool{}

	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			if decl, ok := node.(*ast.GenDecl); ok && decl.Tok == token.CONST {
				for _, spec := range decl.Specs {
					valueSpec, isValue := spec.(*ast.ValueSpec)
					if !isValue {
						continue
					}

					for _, name := range valueSpec.Names {
						names[name.Name] = true
					}
				}
			}

			return true
		})
	}

	return names
}

// propagate는 대입·변수 선언에서 비밀 값을 받은 이름을 고정점까지 표시한다.
func (s secretScanner) propagate(root ast.Node) {
	for changed := true; changed; {
		changed = false

		ast.Inspect(root, func(node ast.Node) bool {
			switch stmt := node.(type) {
			case *ast.AssignStmt:
				changed = s.taintAssignments(stmt.Lhs, stmt.Rhs) || changed
			case *ast.ValueSpec:
				names := make([]ast.Expr, len(stmt.Names))
				for i, name := range stmt.Names {
					names[i] = name
				}

				changed = s.taintAssignments(names, stmt.Values) || changed
			}

			return true
		})
	}
}

func (s secretScanner) taintAssignments(targets, values []ast.Expr) bool {
	changed := false

	for i, target := range targets {
		ident, ok := target.(*ast.Ident)
		if !ok || ident.Name == "_" || s.tainted[ident.Name] || hasNonSecretSuffix(ident.Name) {
			continue
		}

		var value ast.Expr

		switch {
		case len(values) == len(targets):
			value = values[i]
		case len(values) == 1:
			// mac, err := h.computeMAC(body)처럼 다중 반환 호출의 결과다.
			value = values[0]
		default:
			continue
		}

		// 임의 호출의 인자까지 따라가면 canonical 문자열·HTTP 응답도 비밀 값이 되므로
		// 대입 전파는 복사·변환·비밀 이름 호출만 따라간다.
		if s.isSecret(value, false) {
			s.tainted[ident.Name] = true
			changed = true
		}
	}

	return changed
}

func (s secretScanner) comparisons(root ast.Node) []token.Pos {
	var positions []token.Pos

	ast.Inspect(root, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.BinaryExpr:
			if (value.Op == token.EQL || value.Op == token.NEQ) && s.comparesSecret(value.X, value.Y) {
				positions = append(positions, value.Pos())
			}
		case *ast.SwitchStmt:
			positions = append(positions, s.switchComparisons(value)...)
		}

		return true
	})

	return positions
}

// switch tag { case v: }는 tag == v 비교다.
func (s secretScanner) switchComparisons(stmt *ast.SwitchStmt) []token.Pos {
	if stmt.Tag == nil {
		return nil
	}

	var positions []token.Pos

	for _, clause := range stmt.Body.List {
		caseClause, ok := clause.(*ast.CaseClause)
		if !ok {
			continue
		}

		for _, expr := range caseClause.List {
			if s.comparesSecret(stmt.Tag, expr) {
				positions = append(positions, expr.Pos())
			}
		}
	}

	return positions
}

func (s secretScanner) comparesSecret(left, right ast.Expr) bool {
	if s.isNamedConstant(left) || s.isNamedConstant(right) {
		return false
	}

	if isExistenceLiteral(left) && !isCall(right) || isExistenceLiteral(right) && !isCall(left) {
		return false
	}

	return s.isSecret(left, true) || s.isSecret(right, true)
}

func (s secretScanner) isNamedConstant(expr ast.Expr) bool {
	switch value := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return s.constants[value.Name]
	case *ast.SelectorExpr:
		pkg, ok := value.X.(*ast.Ident)

		return ok && s.packages[pkg.Name] && s.constants[value.Sel.Name]
	default:
		return false
	}
}

func isExistenceLiteral(expr ast.Expr) bool {
	switch value := ast.Unparen(expr).(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return value.Name == "nil"
	default:
		return false
	}
}

func isCall(expr ast.Expr) bool {
	_, ok := ast.Unparen(expr).(*ast.CallExpr)

	return ok
}

// 식이 비밀 값을 담는지 판정한다. 호출 추적(throughCalls)이 켜지면 strings.Compare(sig, want)처럼
// 비밀 값을 인자로 받는 호출 결과도 비밀 값으로 본다.
func (s secretScanner) isSecret(expr ast.Expr, throughCalls bool) bool {
	switch value := expr.(type) {
	case *ast.Ident:
		if s.constants[value.Name] || s.packages[value.Name] {
			return false
		}

		return s.tainted[value.Name] || isSecretName(value.Name)
	case *ast.SelectorExpr:
		// secret.Value처럼 비밀 값의 하위 필드도 비밀 값이다. secret.Path 같은 메타데이터는 제외한다.
		if s.isNamedConstant(value) {
			return false
		}

		return isSecretName(value.Sel.Name) || !hasNonSecretSuffix(value.Sel.Name) && s.isSecret(value.X, throughCalls)
	case *ast.CallExpr:
		return s.isSecretCall(value, throughCalls)
	case *ast.ParenExpr:
		return s.isSecret(value.X, throughCalls)
	case *ast.IndexExpr:
		return s.isSecret(value.X, throughCalls)
	case *ast.SliceExpr:
		return s.isSecret(value.X, throughCalls)
	case *ast.StarExpr:
		return s.isSecret(value.X, throughCalls)
	case *ast.UnaryExpr:
		return s.isSecret(value.X, throughCalls)
	default:
		return false
	}
}

func (s secretScanner) isSecretCall(call *ast.CallExpr, throughCalls bool) bool {
	if isLengthCall(call) {
		return false
	}

	if s.isSecret(call.Fun, throughCalls) {
		return true
	}

	if !throughCalls && !isConversion(call) {
		return false
	}

	return slices.ContainsFunc(call.Args, func(arg ast.Expr) bool {
		return s.isSecret(arg, throughCalls)
	})
}

// string(sig)·[]byte(sig) 변환은 값을 그대로 옮긴다.
func isConversion(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.ArrayType:
		return true
	case *ast.Ident:
		return fun.Name == "string"
	default:
		return false
	}
}

// len(token) == 0 같은 길이 확인은 값 자체를 비교하지 않는다.
func isLengthCall(call *ast.CallExpr) bool {
	ident, ok := call.Fun.(*ast.Ident)

	return ok && (ident.Name == "len" || ident.Name == "cap")
}

func isSecretName(name string) bool {
	if hasNonSecretSuffix(name) {
		return false
	}

	return slices.ContainsFunc(identifierWords(name), func(word string) bool {
		return slices.Contains(secretWords, word)
	})
}

func hasNonSecretSuffix(name string) bool {
	words := identifierWords(name)

	return len(words) > 0 && slices.Contains(nonSecretSuffixes, words[len(words)-1])
}

func identifierWords(name string) []string {
	words := identifierWordPattern.FindAllString(name, -1)
	for i, word := range words {
		words[i] = strings.ToLower(word)
	}

	return words
}
