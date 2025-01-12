package main

import (
	"github.com/github.com/operator-framework/operator-controller/custom-linters/setuplognilerrorcheck"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/unitchecker"
)

// Define the custom Linters implemented in the project
var customLinters = []*analysis.Analyzer{
	setuplognilerrorcheck.SetupLogNilErrorCheck,
}

func main() {
	unitchecker.Main(customLinters...)
}
