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

show(1.5 + 1.5);
show(0.1 * 3);
show(1.0 - 1);
show(10 / 4.0);
show(5.0 / 1);
show(1e308 * 10);
show(-1e308 * 10);
$inf = 1e308 * 10;
show($inf - $inf);
show($inf * 0);
show(0.0 * -1);
show(2.5 ** 2);
show(4 ** 0.5);
show((-8) ** (1 / 3));
show(1.5 + 1);
show(7.5 % 2);
show(-7.9 % 3);
show(10 % 3.0);
