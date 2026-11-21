<?php
declare(strict_types=1);
// Strict mode: a closure with an int parameter does not accept the string "5".
// The matching calls run first; the string call raises an uncaught type error.

$square = function (int $n): int {
    return $n * $n;
};

echo "int argument: ", $square(3), "\n";

$half = function (float $f): float {
    return $f / 2;
};
echo "int widened to float: ", $half(5), "\n";

$label = function (string $s): string {
    return "[" . $s . "]";
};
echo "string argument: ", $label("ok"), "\n";

echo "about to pass a string to the int parameter\n";

echo "result: ", $square("5"), "\n";

echo "not reached\n";
