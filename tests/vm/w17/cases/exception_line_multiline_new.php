<?php
// exception_line_multiline_new: which line getLine reports for a new Exception spread over several lines
function wrap($a, $b) {
    return new RuntimeException(
        "wrapped " . $a
        . " " . $b,
        4
    );
}

function pass_through($value, $error) {
    return $error;
}

$e1 = new Exception(
    "spread",
    2
);
echo "spread_line: ", $e1->getLine(), "\n";

$e2 = wrap(
    "x",
    "y"
);
echo "wrap_line: ", $e2->getLine(), "\n";

$e3 = pass_through(
    1,
    new Exception(
        "argument"
    )
);
echo "nested_arg_line: ", $e3->getLine(), "\n";

$e4 = pass_through(1, new Error(
    "same line open"
));
echo "open_paren_line: ", $e4->getLine(), "\n";

$e5 = new LogicException("one line");
echo "one_line: ", $e5->getLine(), "\n";
echo "spread_message: ", $e1->getMessage(), "\n";
echo "wrap_message: ", $e2->getMessage(), "\n";
echo "wrap_code: ", $e2->getCode(), "\n";
echo "nested_message: ", $e3->getMessage(), "\n";
echo "lines_increase: ", ($e5->getLine() > $e1->getLine() ? 1 : 0), "\n";
