package closerleak

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestCloserLeak(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, Analyzer, "testcases")
}
