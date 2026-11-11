<?php
function b($r)
{
    return $r ? "T" : "F";
}
$n = null;
$z = 0;
$e = "";
$a = ["k" => null, "v" => 1, "f" => false, "nested" => ["x" => 1]];
echo b(isset($n)), b(isset($z)), b(isset($e)), b(isset($undef)), "\n";
echo b(isset($a["k"])), b(isset($a["v"])), b(isset($a["f"])), b(isset($a["no"])), "\n";
echo b(isset($a["nested"]["x"])), b(isset($a["nested"]["y"])), b(isset($a["v"]["x"])), "\n";
echo b(isset($z, $e)), b(isset($z, $n)), b(isset($undef["x"])), "\n";
unset($z);
echo b(isset($z)), "\n";
echo $z, "|\n";
unset($a["v"]);
echo b(isset($a["v"])), "\n";
echo $a["v"], "|\n";
unset($never);
unset($a["never"]);
unset($a["nested"]["x"]);
echo b(isset($a["nested"])), b(isset($a["nested"]["x"])), "\n";
$s = "str";
echo b(isset($s)), "\n";
