<?php
foreach ([1, "s", null, true, 1.5, [2]] as [$z]) {
    echo $z === null ? "null" : $z, "\n";
}
foreach ([[1], 5, [6]] as $k => ['0' => $r]) {
    echo $k, $r === null ? "null" : $r, "\n";
}
foreach ([[[1]], [2]] as [[$n]]) {
    echo $n === null ? "null" : $n, "\n";
}
