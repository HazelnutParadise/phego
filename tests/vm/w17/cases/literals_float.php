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

show(1.5);
show(.5);
show(1.);
show(1e3);
show(1E-7);
show(-2.5e+3);
show(0.1 + 0.2);
show(1 / 3);
show(1.0);
show(-0.0);
show(100.0);
show(1e14);
show(1e15);
show(123456789012345.678);
show(12345678901234567890);
