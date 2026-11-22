<?php
$i = 10;
$f = fn() => $i++;
echo $f(), $f(), $i, "\n";
$g = fn() => [$i, $j ?? "unset"];
echo $g()[0], $g()[1], "\n";
$h = fn($x) => $x + $undefined;
echo $h(1), "\n";
$k = fn() => fn() => $zz;
echo $k()() ?? "n", "\n";
