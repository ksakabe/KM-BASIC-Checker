package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/kmbasic-checker/internal/checker"
)

const version = "0.1.0"
const defaultLibraryDir = "/Users/sakabe/Downloads/MachiKania/1.7.0/machikania-p/LIB"

func main() {
	var (
		format      string
		target      string
		libraryDir  string
		showVersion bool
	)

	flag.StringVar(&format, "format", "text", "出力形式: text または json")
	flag.StringVar(&target, "target", "type-pu", "対象機種: type-z, type-m, type-p, type-pu")
	flag.StringVar(&libraryDir, "lib", defaultLibraryDir, "KM-BASICクラスライブラリのディレクトリ")
	flag.BoolVar(&showVersion, "version", false, "バージョンを表示")
	flag.Parse()

	if showVersion {
		fmt.Println("kmbasic-check", version)
		return
	}

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "使い方: kmbasic-check [オプション] FILE.BAS [...]")
		os.Exit(2)
	}

	exitCode := 0
	allResults := make([]checker.FileResult, 0, flag.NArg())

	for _, name := range flag.Args() {
		options := checker.Options{Target: target}
		if libraryDir != "" {
			options.LibraryDirs = []string{libraryDir}
		}
		result, err := checker.CheckFile(name, options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			exitCode = 2
			continue
		}
		allResults = append(allResults, result)
		if result.ErrorCount() > 0 {
			exitCode = 1
		}
	}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(allResults); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "text":
		for _, result := range allResults {
			printText(result)
		}
	default:
		fmt.Fprintf(os.Stderr, "不明な出力形式です: %s\n", format)
		os.Exit(2)
	}

	os.Exit(exitCode)
}

func printText(result checker.FileResult) {
	if len(result.Diagnostics) == 0 {
		fmt.Printf("%s: 問題は見つかりませんでした\n", filepath.Clean(result.Path))
		return
	}

	for _, d := range result.Diagnostics {
		fmt.Printf("%s:%d:%d: %s %s: %s\n",
			filepath.Clean(result.Path), d.Line, d.Column,
			d.Severity, d.Code, d.Message)
	}
	fmt.Printf("%s: error=%d warning=%d\n",
		filepath.Clean(result.Path), result.ErrorCount(), result.WarningCount())
}
