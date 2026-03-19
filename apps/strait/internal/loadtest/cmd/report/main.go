//go:build loadtest

// Command report generates HTML and JSON reports from load test results.
//
// Usage:
//
//	go run -tags=loadtest ./internal/loadtest/cmd/report \
//	  -input loadtest-results/latest/ \
//	  -output loadtest-results/latest/ \
//	  -html report.html -json report.json
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
	flag.Parse()

	if *outputDir == "" {
		*outputDir = *inputDir
	}

	gen := loadtest.NewReportGenerator(*inputDir, *outputDir, *htmlFile, *jsonFile)
	if err := gen.Generate(); err != nil {
		log.Fatalf("report generation failed: %v", err)
	}

	fmt.Printf("Reports generated in %s\n", *outputDir)
}
