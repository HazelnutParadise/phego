<?php
function sum($x)
{
    $t = 0;
    foreach ($x as $v) {
        $t += $v;
    }
    return $t;
}
echo sum([1, 2]), "\n";
echo sum(null), "\n";
echo sum("abc"), "\n";
echo $undefined["k"] + sum([4]), "\n";
echo 1 % sum([]), "\n";
