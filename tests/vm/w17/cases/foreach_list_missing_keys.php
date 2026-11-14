<?php
foreach ([[1], [1, 2], []] as [$a, $b]) {
    echo $a === null ? "null" : $a, $b === null ? "null" : $b, "\n";
}
foreach ([['x' => 1], ['y' => 2]] as ['x' => $x,
                                      'y' => $y]) {
    echo $x === null ? "null" : $x, $y === null ? "null" : $y, "\n";
}
foreach ([[[1]], [[]]] as [[$c,
    $d]]) {
    echo $c === null ? "null" : $c, $d === null ? "null" : $d, "\n";
}
