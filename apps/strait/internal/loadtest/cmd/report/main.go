//go:build loadtest

// Command report generates HTML, JSON, and PDF reports from load test results.
//
// Usage:
//
//	go run -tags=loadtest ./internal/loadtest/cmd/report \
//	  -input loadtest-results/latest/ \
//	  -html report.html -json report.json -pdf report.pdf
//
//	# Compare two runs:
//	go run -tags=loadtest ./internal/loadtest/cmd/report \
//	  -input loadtest-results/run-a/ \
//	  -diff loadtest-results/run-b/
package main

import (
	"flag"
	"fmt"
	"log"

	"strait/internal/loadtest"
)

func main() {
	inputDir := flag.String("input", "loadtest-results/latest", "path to load test results directory")
	outputDir := flag.String("output", "", "path to write reports (defaults to input dir)")
	htmlFile := flag.String("html", "report.html", "HTML report filename")
	jsonFile := flag.String("json", "report.json", "JSON report filename")
	pdfFile := flag.String("pdf", "", "PDF report filename (requires Chrome/Chromium)")
	diffDir := flag.String("diff", "", "path to second results dir for comparison")
	flag.Parse()

	if *outputDir == "" {
		*outputDir = *inputDir
	}

	gen := loadtest.NewReportGenerator(*inputDir, *outputDir, *htmlFile, *jsonFile)
	gen.PDFFilename = *pdfFile
	gen.DiffDir = *diffDir

	if err := gen.Generate(); err != nil {
		log.Fatalf("report generation failed: %v", err)
	}

	fmt.Printf("Reports generated in %s\n", *outputDir)
}
