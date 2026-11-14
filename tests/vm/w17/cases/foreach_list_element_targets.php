<?php
$out = [];
$m = [];
foreach ([[1, 'a'], [2, 'b']] as [$out[], $m['x']]) {
    echo $m['x'], "\n";
}
echo $out[0], $out[1], $m['x'], "\n";
$k = 0;
foreach ([[1], [2]] as $idx => [$t[$idx][$k]]) {
}
echo $t[0][0], $t[1][0], "\n";
$u = null;
foreach ([['k' => 'v']] as ['k' => $u['deep'][0]]) {
}
echo $u['deep'][0], "\n";
