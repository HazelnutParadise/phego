<?php
$f = false;
$f[0] = 1;
echo "a\n";
$g = false;
[&$a] = $g;
echo "b\n";
$h = false;
$r = &$h;
[&$b] = $h;
echo "c\n";
$i = ['k' => false];
[&$c] = $i['k'];
echo "d\n";
$j = false;
[[&$d]] = $j;
echo "e\n";
$rows = [false];
foreach ($rows as [&$e]) {}
echo "f\n";
$a = 'A'; $b = 'B'; $c = 'C'; $d = 'D'; $e = 'E';
echo $g[0], $h[0], $i['k'][0], $j[0][0], $rows[0][0], "\n";
