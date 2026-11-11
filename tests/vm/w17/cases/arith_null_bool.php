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

show(null + 1);
show(null - null);
show(true + true);
show(false * 3);
show(true / 2);
show(null * 2.5);
show(7 % true);
show(true . "|" . false . "|" . null);
show(null . null);
show(1 + 1.0);
show(true - 0.5);
show(-null);
show(-true);
show(+false);
show(!null);
show(!"0");
show(!"0.0");
show(![]);
show(![0]);
