package phego_test

import (
	"context"
	"errors"
	"fmt"

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
