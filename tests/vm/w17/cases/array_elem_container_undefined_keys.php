<?php
$a = [];
$a[$k] = 5;
echo $a[""], "\n";
$a[$k] += 1;
echo $a[""], "\n";
$a[$j]++;
echo $a[""], "\n";
$b[$m][$n] = "both undefined";
echo $b[""][""], "\n";
$c = [1, 2];
$c[$p] ??= "unused";
echo $c[""], "\n";
unset($c[$q]);
echo isset($c[""]) ? "has" : "gone", "\n";
echo $a[$r] . "|", "\n";
