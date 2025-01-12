package main

import (
	"github.com/operator-framework/operator-controller/hack/ci/custom-linters/analyzers"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/unitchecker"
)

// Define the custom Linters implemented in the project
var customLinters = []*analysis.Analyzer{
	analyzers.SetupLogNilErrorCheck,
}

func main() {
	unitchecker.Main(customLinters...)
}
