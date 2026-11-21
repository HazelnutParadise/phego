<?php
declare(strict_types=1);

function i(int $x): int
{
    return $x;
}
function f(float $x): float
{
    return $x;
}
function n(?string $x = null): string
{
    return $x ?? "null";
}
function u(int|string $x): string
{
    return "u:" . $x;
}

echo i(5), "\n";
echo "int to float allowed: ", f(5), " is float: ", (f(5) === 5.0 ? "yes" : "no"), "\n";
echo n(), " ", n(null), " ", n("s"), "\n";
echo u(1), " ", u("a"), "\n";

class Strict
{
    public function m(bool $b, array $a, string ...$rest): string
    {
        $out = ($b ? "T" : "F") . count_items($a);
        foreach ($rest as $r) {
            $out .= $r;
        }
        return $out;
    }
}
function count_items(array $a): int
{
    $n = 0;
    foreach ($a as $v) {
        $n++;
    }
    return $n;
}
echo (new Strict)->m(true, [1, 2, 3], "x", "y"), "\n";
function ret_float(): float
{
    return 1;
}
echo "return int as float: ", ret_float(), " ", (ret_float() === 1.0 ? "float" : "int"), "\n";
