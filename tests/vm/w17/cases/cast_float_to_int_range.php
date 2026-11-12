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

show((int)1e20);
show((int)-1e20);
show((int)9.2233720368547758E+18);
show((int)2147483648.0);
show((int)4294967296.5);
$inf = 1e308 * 10;
show((int)$inf);
show((int)($inf - $inf));
show((int)"1e20");
show((int)"9999999999999999999");
