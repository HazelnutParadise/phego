<?php
// A closure with two required parameters is called with one argument.
// PHP raises argument count error, and the message names the closure.

$add = function (int $a, int $b) {
    return $a + $b;
};

echo "two arguments: ", $add(2, 3), "\n";

$defaulted = function ($a, $b = 9) {
    return $a . "/" . $b;
};
echo "defaulted with one: ", $defaulted("x"), "\n";

$variadic = function ($first, ...$rest) {
    return $first;
};
echo "variadic with one: ", $variadic(4), "\n";

echo "about to call with too few arguments\n";

$result = $add(1);

echo "not reached: ", $result, "\n";
