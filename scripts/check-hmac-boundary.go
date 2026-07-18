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
	"strconv"
	"strings"
)

const (
	allowedSignerCallFile         = "internal/client/transport/client.go"
	allowedSignerDefFile          = "internal/client/signing/signer.go"
	allowedVerifierSignerCallFile = "webhook/handler_options.go"
	allowedPublicSignerCallFile   = "webhooksign/sign.go"
	irisHMACImportPath            = "github.com/park285/iris-client-go/internal/irishmac"
	maxSignerCalls                = 2
)

// 공개 helper는 signer 생성 허용 범위를 넓히지 않고 경계만 이동한다.

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
	var findings []violation
	var signerCalls []token.Position

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
			return relErr
		}
		rel = filepath.ToSlash(rel)

		fileFindings, fileCalls := inspectFile(fset, file, rel)
		findings = append(findings, fileFindings...)
		signerCalls = append(signerCalls, fileCalls...)
		return nil
	})
	if err != nil {
		return nil, err
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

func inspectFile(fset *token.FileSet, file *ast.File, rel string) ([]violation, []token.Position) {
	findings := inspectIrisHMACImports(fset, file, rel)
	var signerCalls []token.Position
	exempt := make(map[*ast.Ident]struct{})

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Name == nil {
				return true
			}
			switch node.Name.Name {
			case "signIrisRequest":
				findings = append(findings, violation{
					pos: fset.Position(node.Name.Pos()),
					msg: "signIrisRequest must remain test-only",
				})
			case "newHMACSigner", "NewHMACSigner":
				exempt[node.Name] = struct{}{}
				if rel != allowedSignerDefFile {
					findings = append(findings, violation{
						pos: fset.Position(node.Name.Pos()),
						msg: fmt.Sprintf("%s definition is restricted to %s", node.Name.Name, allowedSignerDefFile),
					})
				}
			}
		case *ast.CallExpr:
			if ident := signerConstructorIdent(node.Fun); ident != nil {
				exempt[ident] = struct{}{}
				pos := fset.Position(ident.Pos())
				signerCalls = append(signerCalls, pos)
				if rel != allowedSignerCallFile {
					findings = append(findings, violation{
						pos: pos,
						msg: fmt.Sprintf("%s production call sites are restricted to %s", ident.Name, allowedSignerCallFile),
					})
				}
			}
			if ident := irisMACSignerConstructorIdent(node.Fun); ident != nil &&
				rel != allowedVerifierSignerCallFile && rel != allowedPublicSignerCallFile {
				findings = append(findings, violation{
					pos: fset.Position(ident.Pos()),
					msg: fmt.Sprintf("irishmac.NewSigner production calls are restricted to %s and %s", allowedVerifierSignerCallFile, allowedPublicSignerCallFile),
				})
			}
		}
		return true
	})

	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || !isSignerConstructorName(ident.Name) {
			return true
		}
		if _, ok := exempt[ident]; ok {
			return true
		}
		findings = append(findings, violation{
			pos: fset.Position(ident.Pos()),
			msg: ident.Name + " must not escape as a production function value",
		})
		return true
	})

	return findings, signerCalls
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
