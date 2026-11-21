<?php
class A
{
    public $v = 1;
}
class EmptyA
{
}

$o = new A;
$e = new EmptyA;
echo "if object: ", ($o ? "truthy" : "falsy"), "\n";
echo "if empty object: ", ($e ? "truthy" : "falsy"), "\n";
echo "not: ", (!$o ? "true" : "false"), "\n";
echo "bool cast: ", ((bool)$e ? "true" : "false"), "\n";
echo "empty(): ", (empty($o) ? "yes" : "no"), (empty($e) ? "yes" : "no"), "\n";
echo "and/or: ", ($o && $e ? "both" : "no"), " ", ($o || false ? "either" : "no"), "\n";
echo "xor: ", ($o xor $e ? "one" : "zero or two"), "\n";
echo "null coalesce keeps object: ", (($o ?? "none")->v), "\n";
echo "ternary short: ", (($e ?: "fallback") === $e ? "kept" : "fallback"), "\n";

echo "int cast: ", (int)$o, "\n";
echo "float cast: ", (float)$o, "\n";
echo "string concat with int cast: ", "n" . (int)$e, "\n";
echo "array cast is fine: ", ((array)$o)["v"], "\n";
echo "loose compare with 1: ", ($o == 1 ? "equal" : "not"), "\n";
echo "loose compare with 2: ", ($o == 2 ? "equal" : "not"), "\n";
echo "compare with string: ", ($o == "x" ? "equal" : "not"), "\n";
echo "compare with array: ", ($o == [] ? "equal" : "not"), "\n";
echo "greater than int: ", ($o > 0 ? "yes" : "no"), "\n";
