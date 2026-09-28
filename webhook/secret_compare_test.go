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
	"strings"
	"testing"
)

// 식별자를 camelCase·snake_case 단어로 나눈다(bodySHA256 → body, SHA256).
var identifierWordPattern = regexp.MustCompile(`[A-Z]+\d*(?:[a-z]+\d*)?|[a-z]+\d*|\d+`)

// 식별자의 마지막 단어가 다음 중 하나이면 타이밍 공격 대상인 비밀 값으로 본다.
// 예를 들어 tokenErr·hmacImportPath는 마지막 단어가 다르므로 비밀 값이 아니다.
var secretWords = []string{"signature", "sig", "token", "secret", "digest", "sha256", "mac", "hmac"}

// 비밀 값은 subtle.ConstantTimeCompare(constantTimeEqualString)로만 비교한다.
// 빈 값·nil 같은 리터럴과의 비교는 존재 확인이라 허용한다.
func TestSecretValuesAreNotComparedWithEqualityOperators(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()

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

		for _, position := range secretEqualityComparisons(file) {
			t.Errorf("%s: compare secret values with constantTimeEqualString, not ==/!=", fileSet.Position(position))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("scan module sources: %v", err)
	}
}

// scripts/는 저장소 도구라 런타임 비밀 값을 다루지 않는다.
func skipNonSourceDir(name string) error {
	if name == "testdata" || name == "vendor" || name == "scripts" || strings.HasPrefix(name, ".") && name != ".." {
		return filepath.SkipDir
	}

	return nil
}

func secretEqualityComparisons(file *ast.File) []token.Pos {
	var positions []token.Pos

	ast.Inspect(file, func(node ast.Node) bool {
		binary, ok := node.(*ast.BinaryExpr)
		if !ok || binary.Op != token.EQL && binary.Op != token.NEQ {
			return true
		}

		if isLiteralOperand(binary.X) || isLiteralOperand(binary.Y) {
			return true
		}

		if isSecretOperand(binary.X) || isSecretOperand(binary.Y) {
			positions = append(positions, binary.Pos())
		}

		return true
	})

	return positions
}

func isLiteralOperand(expr ast.Expr) bool {
	switch value := expr.(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return value.Name == "nil"
	default:
		return false
	}
}

func isSecretOperand(expr ast.Expr) bool {
	switch value := expr.(type) {
	case *ast.Ident:
		return isSecretName(value.Name)
	case *ast.SelectorExpr:
		return isSecretName(value.Sel.Name)
	case *ast.CallExpr:
		return isSecretOperand(value.Fun) || slices.ContainsFunc(value.Args, isSecretOperand)
	case *ast.ParenExpr:
		return isSecretOperand(value.X)
	default:
		return false
	}
}

func isSecretName(name string) bool {
	words := identifierWordPattern.FindAllString(name, -1)
	if len(words) == 0 {
		return false
	}

	return slices.Contains(secretWords, strings.ToLower(words[len(words)-1]))
}
