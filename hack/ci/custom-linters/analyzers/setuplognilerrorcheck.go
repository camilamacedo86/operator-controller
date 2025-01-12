package analyzers

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

var SetupLogNilErrorCheck = &analysis.Analyzer{
	Name: "setuplognilerrorcheck",
	Doc:  "Detects improper usage of logger.Error() calls, ensuring the first argument is a non-nil error.",
	Run:  runSetupLogNilErrorCheck,
}

func runSetupLogNilErrorCheck(pass *analysis.Pass) (interface{}, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			callExpr, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Ensure function being called is logger.Error
			selectorExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
			if !ok || selectorExpr.Sel.Name != "Error" {
				return true
			}

			// Ensure receiver (logger) is identified
			ident, ok := selectorExpr.X.(*ast.Ident)
			if !ok {
				return true
			}

			// Verify if the receiver is logr.Logger
			obj := pass.TypesInfo.ObjectOf(ident)
			if obj == nil {
				return true
			}

			named, ok := obj.Type().(*types.Named)
			if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "github.com/go-logr/logr" || named.Obj().Name() != "Logger" {
				return true
			}

			if len(callExpr.Args) == 0 {
				return true
			}

			// Check if the first argument of the error log is nil
			firstArg, ok := callExpr.Args[0].(*ast.Ident)
			if !ok || firstArg.Name != "nil" {
				return true
			}

			// Extract error message for the suggestion
			suggestedError := "errors.New(\"kind error (i.e. configuration error)\")"
			suggestedMessage := "\"error message describing the failed operation\""

			if len(callExpr.Args) > 1 {
				if msgArg, ok := callExpr.Args[1].(*ast.BasicLit); ok && msgArg.Kind == token.STRING {
					suggestedMessage = msgArg.Value
				}
			}

			// Get the actual source code line where the issue occurs
			var srcBuffer bytes.Buffer
			if err := format.Node(&srcBuffer, pass.Fset, callExpr); err == nil {
				sourceLine := srcBuffer.String()
				pass.Reportf(callExpr.Pos(),
					"Incorrect usage of 'logger.Error(nil, ...)'. The first argument must be a non-nil 'error'. "+
						"Passing 'nil' results in silent failures, making debugging harder.\n\n"+
						"\U0001F41B **What is wrong?**\n"+
						fmt.Sprintf("   %s\n\n", sourceLine)+
						"\U0001F4A1 **How to solve? Return the error, i.e.:**\n"+
						fmt.Sprintf("   logger.Error(%s, %s, \"key\", value)\n\n", suggestedError, suggestedMessage))
			}

			return true
		})
	}
	return nil, nil
}
