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
	"strings"
)

const (
	allowedSignerCallFile = "internal/client/client.go"
	allowedSignerDefFile  = "internal/client/hmac_signer.go"
	maxSignerCalls        = 2
)

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
				msg: fmt.Sprintf("newHMACSigner production calls are restricted to %s (max %d)", allowedSignerCallFile, maxSignerCalls),
			})
		}
	}
	return findings, nil
}

func inspectFile(fset *token.FileSet, file *ast.File, rel string) ([]violation, []token.Position) {
	var findings []violation
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
			case "newHMACSigner":
				exempt[node.Name] = struct{}{}
				if rel != allowedSignerDefFile {
					findings = append(findings, violation{
						pos: fset.Position(node.Name.Pos()),
						msg: fmt.Sprintf("newHMACSigner definition is restricted to %s", allowedSignerDefFile),
					})
				}
			}
		case *ast.CallExpr:
			if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "newHMACSigner" {
				exempt[ident] = struct{}{}
				pos := fset.Position(ident.Pos())
				signerCalls = append(signerCalls, pos)
				if rel != allowedSignerCallFile {
					findings = append(findings, violation{
						pos: pos,
						msg: fmt.Sprintf("newHMACSigner production call sites are restricted to %s", allowedSignerCallFile),
					})
				}
			}
		}
		return true
	})

	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || ident.Name != "newHMACSigner" {
			return true
		}
		if _, ok := exempt[ident]; ok {
			return true
		}
		findings = append(findings, violation{
			pos: fset.Position(ident.Pos()),
			msg: "newHMACSigner must not escape as a production function value",
		})
		return true
	})

	return findings, signerCalls
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
