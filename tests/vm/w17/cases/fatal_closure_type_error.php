<?php
// A closure with an int parameter is called with an array.
// Weak mode does not convert arrays to int, so PHP raises type error.

$double = function (int $n) {
    return $n * 2;
};

echo "int argument: ", $double(4), "\n";

$numeric = function (int $n) {
    return $n + 1;
};
echo "numeric string in weak mode: ", $numeric("5"), "\n";

$flag = function (bool $b) {
    return $b ? "true" : "false";
};
echo "bool from int: ", $flag(1), "\n";

echo "about to pass an array\n";

echo "result: ", $double([1, 2]), "\n";

echo "not reached\n";
