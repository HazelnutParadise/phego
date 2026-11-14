<?php
$rows = [[1, 2], [3, 4]];
foreach ($rows as [$a, $b]) {
    echo $a, $b, "\n";
}
foreach ($rows as $k => [$c, $d]) {
    echo $k, $c, $d, "\n";
}
$recs = [['id' => 7, 'n' => 'x'], ['id' => 8, 'n' => 'y']];
foreach ($recs as ['id' => $id, 'n' => $n]) {
    echo $id, $n, "\n";
}
foreach ($recs as $k => ['id' => $id]) {
    echo $k, $id, "\n";
}
foreach ([[1, [2, 3]], [4, [5, 6]]] as [$e, [$f, $g]]) {
    echo $e, $f, $g, "\n";
}
foreach ([[1, 2, 3]] as [, , $h]) {
    echo $h, "\n";
}
foreach ([[1, 2]] as list($i, $j)) {
    echo $i, $j, "\n";
}
foreach ([] as [$u]) {
    echo "never\n";
}
echo isset($u) ? "?" : "unset", "\n";
foreach ($rows as [$a]) {
    if ($a === 3) { break; }
    echo $a, "\n";
}
echo $a, $b, "\n";
