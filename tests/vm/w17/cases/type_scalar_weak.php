<?php
function i(int $x)
{
    return "int " . $x;
}
function f(float $x)
{
    return "float " . $x;
}
function s(string $x)
{
    return "string " . $x;
}
function b(bool $x)
{
    return "bool " . ($x ? "true" : "false");
}

echo i(5), " | ", i("42"), " | ", i(" 7"), " | ", i("8 "), " | ", i(3.0), " | ", i(true), " | ", i(false), "\n";
echo i("1e2"), " | ", i("0x1A" === "0x1A" ? 26 : 0), " | ", i(-0.0), "\n";
echo f(1), " | ", f("2.5"), " | ", f("3"), " | ", f(true), " | ", f("1e3"), "\n";
echo s(12), " | ", s(1.5), " | ", s(true), " | [", s(false), "] | ", s(-0.0), " | ", s(1e100), "\n";
echo b(0), " | ", b(1), " | ", b("0"), " | ", b(""), " | ", b("a"), " | ", b(0.0), " | ", b(0.1), "\n";

class Conv
{
    public function m(int $a, string $b, float $c, bool $d)
    {
        return $a . "/" . $b . "/" . $c . "/" . ($d ? "T" : "F");
    }
    public static function sm(int $n)
    {
        return $n * 2;
    }
}
echo (new Conv)->m("1", 2, "3.5", "yes"), "\n";
echo Conv::sm("21"), "\n";

function ret_int($v): int
{
    return $v;
}
function ret_string($v): string
{
    return $v;
}
function ret_float($v): float
{
    return $v;
}
function ret_bool($v): bool
{
    return $v;
}
echo "return coercion: ", ret_int("9"), " ", ret_string(10), " ", ret_float(2), " ", (ret_bool(1) ? "true" : "false"), "\n";
echo "return int from integral float: ", ret_int(4.0), "\n";
echo "return float type is float: ", (ret_float(2) === 2.0 ? "yes" : "no"), "\n";
