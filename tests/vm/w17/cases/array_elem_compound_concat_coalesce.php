<?php
$a = ["s" => "x", "i" => 5, "n" => null, "f" => 1.5, "b" => true];
$a["s"] .= "y";
echo $a["s"], "\n";
$a["i"] .= "0";
echo $a["i"], "\n";
$a["f"] .= "!";
echo $a["f"], "\n";
$a["b"] .= "?";
echo $a["b"], "\n";
$a["n"] .= "z";
echo $a["n"], "\n";
$a["s"] .= $a["i"] . $a["s"];
echo $a["s"], "\n";
$a["n"] ??= "fill";
echo $a["n"], "\n";
$a["n"] ??= "again";
echo $a["n"], "\n";
$a["new"] ??= "created";
echo $a["new"], "\n";
$a["s"] ??= "kept";
echo $a["s"], "\n";
$a["zero"] = 0;
$a["zero"] ??= 99;
echo $a["zero"], "\n";
function noisy()
{
    echo "noisy called\n";
    return "r";
}
$a["s"] ??= noisy();
$a["other"] ??= noisy();
echo $a["other"], "\n";
