package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	allowedSignerCallFile = "internal/client/transport/client.go"
	allowedSignerDefFile  = "internal/client/signing/signer.go"
	irisHMACImportPath    = "github.com/park285/iris-client-go/v2/internal/irishmac"
	maxSignerCalls        = 2
)

// 공개 helper는 signer 생성 허용 범위를 넓히지 않고 경계만 이동한다.

// signing/signer.go가 여기 있는 것은 그 파일이 irishmac.Signer를 그대로 위임하기 때문이다.
// 위임을 막으면 signing 쪽이 gate가 검사하지 못하는 두 번째 HMAC 구현을 들게 되므로,
// 축자 복제본보다 이 호출 하나를 허용하는 편이 경계를 좁게 유지한다.
var allowedIrisMACSignerCallFiles = map[string]struct{}{
	"internal/client/signing/signer.go": {},
	"webhook/handler_options.go":        {},
	"webhooksign/sign.go":               {},
}

var allowedIrisHMACImportFiles = map[string]struct{}{
	"internal/client/signing/canonical.go":   {},
	"internal/client/signing/headers.go":     {},
	"internal/client/signing/signer.go":      {},
	"internal/client/transport/constants.go": {},
	"internal/client/transport/path.go":      {},
	"webhook/constants.go":                   {},
	"webhook/handler.go":                     {},
	"webhook/handler_options.go":             {},
	"webhook/handler_validation.go":          {},
	"webhooksign/sign.go":                    {},
}

type violation struct {
	pos token.Position
	msg string
}

func main() {
	root := flag.String("root", ".", "repository root")

	flag.Parse()

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve root: %v\n", err)
		os.Exit(2)
	}

	findings, err := scan(absRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}

	if len(findings) > 0 {
		for _, finding := range findings {
			fmt.Fprintf(os.Stderr, "%s: %s\n", relPosition(absRoot, finding.pos), finding.msg)
		}

		os.Exit(1)
	}

	fmt.Println("ok - hmac boundary clean")
}

func scan(root string) ([]violation, error) {
	fset := token.NewFileSet()

	var (
		findings    []violation
		signerCalls []token.Position
	)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			findings = append(findings, violation{pos: token.Position{Filename: path}, msg: fmt.Sprintf("walk error: %v", err)})
			return nil
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules":
				return filepath.SkipDir
			default:
				return nil
			}
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if parseErr != nil {
			findings = append(findings, violation{pos: token.Position{Filename: path}, msg: fmt.Sprintf("parse error: %v", parseErr)})
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("relative path: %w", relErr)
		}

		rel = filepath.ToSlash(rel)

		fileFindings, fileCalls := inspectFile(fset, file, rel)

		findings = append(findings, fileFindings...)
		signerCalls = append(signerCalls, fileCalls...)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}

	if len(signerCalls) > maxSignerCalls {
		for _, pos := range signerCalls {
			findings = append(findings, violation{
				pos: pos,
				msg: fmt.Sprintf("NewHMACSigner production calls are restricted to %s (max %d)", allowedSignerCallFile, maxSignerCalls),
			})
		}
	}

	return findings, nil
}

type fileInspector struct {
	fset        *token.FileSet
	rel         string
	findings    []violation
	signerCalls []token.Position
	exempt      map[*ast.Ident]struct{}
}

func inspectFile(fset *token.FileSet, file *ast.File, rel string) ([]violation, []token.Position) {
	inspector := &fileInspector{
		fset:     fset,
		rel:      rel,
		findings: inspectIrisHMACImports(fset, file, rel),
		exempt:   make(map[*ast.Ident]struct{}),
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			inspector.inspectFuncDecl(node)
		case *ast.CallExpr:
			inspector.inspectCall(node)
		}

		return true
	})

	ast.Inspect(file, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			inspector.inspectEscapedIdent(ident)
		}

		return true
	})

	return inspector.findings, inspector.signerCalls
}

func (i *fileInspector) inspectFuncDecl(node *ast.FuncDecl) {
	if node.Name == nil {
		return
	}

	switch node.Name.Name {
	case "signIrisRequest":
		i.findings = append(i.findings, violation{
			pos: i.fset.Position(node.Name.Pos()),
			msg: "signIrisRequest must remain test-only",
		})
	case "newHMACSigner", "NewHMACSigner":
		i.exempt[node.Name] = struct{}{}

		if i.rel != allowedSignerDefFile {
			i.findings = append(i.findings, violation{
				pos: i.fset.Position(node.Name.Pos()),
				msg: fmt.Sprintf("%s definition is restricted to %s", node.Name.Name, allowedSignerDefFile),
			})
		}
	}
}

func (i *fileInspector) inspectCall(node *ast.CallExpr) {
	if ident := signerConstructorIdent(node.Fun); ident != nil {
		i.exempt[ident] = struct{}{}

		pos := i.fset.Position(ident.Pos())

		i.signerCalls = append(i.signerCalls, pos)

		if i.rel != allowedSignerCallFile {
			i.findings = append(i.findings, violation{
				pos: pos,
				msg: fmt.Sprintf("%s production call sites are restricted to %s", ident.Name, allowedSignerCallFile),
			})
		}
	}

	if ident := irisMACSignerConstructorIdent(node.Fun); ident != nil {
		if _, ok := allowedIrisMACSignerCallFiles[i.rel]; !ok {
			i.findings = append(i.findings, violation{
				pos: i.fset.Position(ident.Pos()),
				msg: fmt.Sprintf("irishmac.NewSigner production calls are restricted to %s", strings.Join(sortedKeys(allowedIrisMACSignerCallFiles), ", ")),
			})
		}
	}
}

func (i *fileInspector) inspectEscapedIdent(ident *ast.Ident) {
	if !isSignerConstructorName(ident.Name) {
		return
	}

	if _, ok := i.exempt[ident]; ok {
		return
	}

	i.findings = append(i.findings, violation{
		pos: i.fset.Position(ident.Pos()),
		msg: ident.Name + " must not escape as a production function value",
	})
}

func inspectIrisHMACImports(fset *token.FileSet, file *ast.File, rel string) []violation {
	var findings []violation

	for _, importSpec := range file.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil || importPath != irisHMACImportPath {
			continue
		}

		if _, ok := allowedIrisHMACImportFiles[rel]; !ok {
			findings = append(findings, violation{
				pos: fset.Position(importSpec.Path.Pos()),
				msg: fmt.Sprintf("%s imports are restricted to the HMAC boundary allowlist", irisHMACImportPath),
			})
		}
	}

	return findings
}

func signerConstructorIdent(expr ast.Expr) *ast.Ident {
	switch value := expr.(type) {
	case *ast.Ident:
		if isSignerConstructorName(value.Name) {
			return value
		}
	case *ast.SelectorExpr:
		if isSignerConstructorName(value.Sel.Name) {
			return value.Sel
		}
	}

	return nil
}

func isSignerConstructorName(name string) bool {
	return name == "newHMACSigner" || name == "NewHMACSigner"
}

func irisMACSignerConstructorIdent(expr ast.Expr) *ast.Ident {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "NewSigner" {
		return nil
	}

	qualifier, ok := selector.X.(*ast.Ident)
	if !ok || qualifier.Name != "irishmac" {
		return nil
	}

	return selector.Sel
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

func relPosition(root string, pos token.Position) string {
	if pos.Filename == "" {
		return pos.String()
	}

	rel, err := filepath.Rel(root, pos.Filename)
	if err == nil {
		pos.Filename = filepath.ToSlash(rel)
	}

	return pos.String()
}
