package setuplognilerrorcheck

import (
	"go/ast"
	"golang.org/x/tools/go/analysis"
)

var SetupLogNilErrorCheck = &analysis.Analyzer{
	Name: "setuplognilerrorcheck",
	Doc:  "check for setupLog.Error(nil, ...) calls with nil as the first argument",
	Run:  runSetupLogNilErrorCheck,
}

func runSetupLogNilErrorCheck(pass *analysis.Pass) (interface{}, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			expr, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			selectorExpr, ok := expr.Fun.(*ast.SelectorExpr)
			if !ok || selectorExpr.Sel.Name != "Error" {
				return true
			}

			i, ok := selectorExpr.X.(*ast.Ident)
			if !ok || i.Name != "setupLog" {
				return true
			}

			if len(expr.Args) > 0 {
				if arg, ok := expr.Args[0].(*ast.Ident); ok && arg.Name == "nil" {
					pass.Reportf(expr.Pos(), "Avoid using 'setupLog.Error(nil, ...)'. Instead, use 'errors.New()' "+
						"or 'fmt.Errorf()' to ensure logs are created. Using 'nil' for errors can result in silent "+
						"failures, making bugs harder to detect.")
				}
			}
			return true
		})
	}
	return nil, nil
}
