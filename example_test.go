package phego_test

import (
	"context"
	"errors"
	"fmt"
	"testing/fstest"

	"github.com/HazelnutParadise/phego"
)

// The specification's single-function example. Until the VM exists, Run
// reports that execution is unsupported instead of inventing a result.
func ExampleRun() {
	result, err := phego.Run(context.Background(), `<?php return 10 + 20;`)
	var unsupported *phego.UnsupportedError
	switch {
	case errors.As(err, &unsupported):
		fmt.Println("not yet executable:", unsupported.Feature)
	case err != nil:
		fmt.Println("error:", err)
	default:
		value, _ := result.Return.Int64()
		fmt.Println(value)
	}
	// Output: not yet executable: PHP execution
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
