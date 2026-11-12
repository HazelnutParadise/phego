<?php
function show($v)
{
    if ($v === null) {
        echo "NULL\n";
    } elseif ($v === true) {
        echo "bool(true)\n";
    } elseif ($v === false) {
        echo "bool(false)\n";
    } elseif ($v !== $v) {
        echo "float(NAN)\n";
    } elseif ((array)$v === $v) {
        echo "array\n";
    } elseif ((float)$v === $v) {
        echo "float(", $v, ")\n";
    } elseif ((string)$v === $v) {
        echo "string(\"", $v, "\")\n";
    } else {
        echo "int(", $v, ")\n";
    }
}

show(7 + 3);
show(7 - 10);
show(6 * 7);
show(6 / 3);
show(7 / 2);
show(-7 / 2);
show(7 % 3);
show(-7 % 3);
show(7 % -3);
show(-7 % -3);
show(0 % 5);
show(2 ** 10);
show(2 ** 62);
show(2 ** 63);
show(2 ** 64);
show((-2) ** 63);
show(-2 ** 2);
show(2 ** -1);
show(3 ** 0);
show(0 ** 0);
show(2 ** 3 ** 2);
show((-3) ** 3);
show(10 - 2 - 3);
show(2 + 3 * 4);
show((2 + 3) * 4);
show(7 + -3);
show(- -5);
