package phego_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing/fstest"

	"github.com/HazelnutParadise/phego"
)

// The specification's single-function example. A construct this build
// cannot run yet is an UnsupportedError with its location, never a guess.
func ExampleRun() {
	result, err := phego.Run(context.Background(), `<?php return 10 + 20;`)
	var unsupported *phego.UnsupportedError
	switch {
	case errors.As(err, &unsupported):
		fmt.Println("not yet executable:", unsupported.Feature, "on line", unsupported.Line)
	case err != nil:
		fmt.Println("error:", err)
	default:
		value, _ := result.Return.Int64()
		fmt.Println(value)
	}
	// Output: 30
}

// Displayed errors go into the output, as PHP prints them; the log channel
// is Result.Diagnostics. This run displays nothing and logs every warning.
func ExampleWithINI() {
	result, err := phego.Run(context.Background(), "<?php\necho \"total: \", $missing + 2, \"\\n\";\n",
		phego.WithINI("display_errors", "0"), phego.WithINI("log_errors", "1"))
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s", result.Output)
	for _, d := range result.Diagnostics {
		fmt.Printf("%s: %s in %s on line %d\n", d.Level, d.Message, d.File, d.Line)
	}
	// Output:
	// total: 2
	// Warning: Undefined variable $missing in Standard input code on line 2
}

// Syntax errors are located like PHP's.
func ExampleEngine_Compile() {
	engine, err := phego.NewEngine(phego.Config{})
	if err != nil {
		panic(err)
	}
	defer engine.Close(context.Background())
	_, err = engine.Compile(context.Background(), "index.php", "<?php\nif (true) {\n")
	var phpErr *phego.PHPError
	if errors.As(err, &phpErr) {
		fmt.Println(phpErr.Kind, "on line", phpErr.Line)
	}
	// Output: ParseError on line 3
}

// A policy grants file trees and network destinations; everything else is
// refused, and the refusal names the capability and rule.
func ExampleConfig_policy() {
	engine, err := phego.NewEngine(phego.Config{
		Policy: phego.Policy{
			Mode:  phego.ModeEnforce,
			Files: []phego.FileGrant{{Path: "/app", FS: fstest.MapFS{"index.php": {Data: []byte("<?php return 1;")}}}},
			Network: []phego.NetworkGrant{
				{Scheme: "https", Host: "api.example.com", Ports: []int{443}},
			},
		},
		Limits:    phego.Limits{Steps: 50_000_000, Memory: 128 << 20},
		MaxActive: 64,
		MaxQueued: 128,
	})
	if err != nil {
		panic(err)
	}
	defer engine.Close(context.Background())
	_, err = engine.RunFile(context.Background(), "/etc/passwd")
	var denied *phego.PolicyError
	if errors.As(err, &denied) {
		fmt.Println(denied.Capability, denied.Rule)
	}
	// Output: file-read outside-root
}

// Use Root for a directory on disk. FS is for trusted, self-contained sources
// such as embedded files; os.DirFS is not a confined directory mount.
func ExampleFileGrant_root() {
	dir, err := os.MkdirTemp("", "phego-root-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php return 1;"), 0600); err != nil {
		panic(err)
	}
	engine, err := phego.NewEngine(phego.Config{Policy: phego.Policy{Files: []phego.FileGrant{{Path: "/app", Root: dir}}}})
	if err != nil {
		panic(err)
	}
	defer engine.Close(context.Background())
	result, err := engine.RunFile(context.Background(), "/app/index.php")
	if err != nil {
		panic(err)
	}
	value, _ := result.Return.Int64()
	fmt.Println("returned", value)
	_, err = engine.RunFile(context.Background(), "/outside.php")
	var denied *phego.PolicyError
	if errors.As(err, &denied) {
		fmt.Println(denied.Capability, denied.Rule)
	}
	// Output:
	// returned 1
	// file-read outside-root
}
