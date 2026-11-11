<?php
echo $nope, "|\n";
$x = $missing + 1;
echo $x, "\n";
echo "in string [$alsoMissing]\n";
function inner()
{
    return $local;
}
$r = inner();
echo $r === null ? "null returned" : "other", "\n";
$global = 5;
function noGlobals()
{
    echo $global, "|\n";
}
noGlobals();
