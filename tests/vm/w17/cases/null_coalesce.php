<?php
$a = ["k" => null, "z" => 0, "n" => ["in" => "deep"]];
echo $undefined ?? "undef", "\n";
echo $a["k"] ?? "null elem", "\n";
echo $a["z"] ?? "zero", "\n";
echo $a["missing"] ?? "missing", "\n";
echo $a["n"]["in"] ?? "no", "\n";
echo $a["n"]["nope"] ?? "nested missing", "\n";
echo $a["z"]["x"] ?? "offset on int", "\n";
echo $undefined["x"] ?? "undef offset", "\n";
echo $nope ?? $a["missing"] ?? "chain", "\n";
$f = false;
echo $f ?? "not used", "|\n";
$e = "";
echo $e ?? "not used either", "|\n";
echo null ?? "literal null", "\n";
