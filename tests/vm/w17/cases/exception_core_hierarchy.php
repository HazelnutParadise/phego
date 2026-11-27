<?php
// exception_core_hierarchy: instanceof matrix for the core Throwable classes
function row($label, $o) {
    echo $label, ": T=", ($o instanceof Throwable ? 1 : 0),
        " E=", ($o instanceof Exception ? 1 : 0),
        " Er=", ($o instanceof Error ? 1 : 0),
        " TE=", ($o instanceof TypeError ? 1 : 0),
        " AE=", ($o instanceof ArithmeticError ? 1 : 0),
        " CE=", ($o instanceof CompileError ? 1 : 0),
        " S=", ($o instanceof Stringable ? 1 : 0),
        "\n";
}

echo "columns: T=Throwable E=Exception Er=Error TE=TypeError AE=ArithmeticError CE=CompileError S=Stringable\n";
row("Exception", new Exception("a"));
row("ErrorException", new ErrorException("a"));
row("Error", new Error("a"));
row("CompileError", new CompileError("a"));
row("ParseError", new ParseError("a"));
row("TypeError", new TypeError("a"));
row("ValueError", new ValueError("a"));
row("ArithmeticError", new ArithmeticError("a"));
row("DivisionByZeroError", new DivisionByZeroError("a"));
row("ArgumentCountError", new ArgumentCountError("a"));
row("UnhandledMatchError", new UnhandledMatchError("a"));

$ee = new ErrorException("severity default");
echo "error_exception_is_exception: ", ($ee instanceof Exception ? 1 : 0), "\n";
echo "error_exception_is_error: ", ($ee instanceof Error ? 1 : 0), "\n";
$pe = new ParseError("p");
echo "parse_error_is_compile: ", ($pe instanceof CompileError ? 1 : 0), "\n";
echo "division_is_arithmetic: ", (new DivisionByZeroError("d") instanceof ArithmeticError ? 1 : 0), "\n";
echo "end\n";
