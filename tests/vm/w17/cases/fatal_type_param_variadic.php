<?php
function sum(int ...$n)
{
    $t = 0;
    foreach ($n as $v) {
        $t += $v;
    }
    return $t;
}
echo sum(1, 2), "\n";
echo sum(1, 2, "x"), "\n";
