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

show(-5);
show(+5);
show(-"5");
show(+"3.0");
show(-" 7");
show(-null);
show(-false);
show(+true);
show(-1.5);
show(-(-0.0));
show(!5);
show(~7);
show(- - - 2);
show(-2 ** 2);
show((-2) ** 2);
show(!!"a");
