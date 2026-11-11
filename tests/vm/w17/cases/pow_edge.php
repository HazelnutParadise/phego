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

show(0 ** -1);
show(0.0 ** -1);
show(0 ** -2);
show((-1) ** 0.5);
show(1 ** 1e300);
show((-1) ** 9223372036854775807);
show(2 ** 0.5);
show(10 ** 20);
show(10 ** 18);
show(10 ** -2);
show("2" ** "3");
show("2.0" ** 2);
show(null ** 2);
show(true ** 5);
show(2 ** false);
$b = 3;
$b **= 2;
show($b);
