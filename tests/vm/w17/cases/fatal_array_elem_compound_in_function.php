<?php
function bump($a, $k)
{
    $a[$k] += 1;
    return $a[$k];
}
echo bump([1], 0), "\n";
echo bump(["x" => 5], "x"), "\n";
echo bump([], "q"), "\n";
echo "before the array operand\n";
echo bump([[]], 0), "\n";
echo "not reached\n";
