<?php
$e = [];
foreach ($e as $v) {
    echo "never\n";
}
echo isset($v) ? "v set" : "v unset", "\n";
foreach ($e as $k => $v) {
    echo "never\n";
}
echo isset($k) ? "k set" : "k unset", "\n";
$v = "keep";
foreach ([] as $v) {
}
echo $v, "\n";
$k = "keepk";
foreach ([] as $k => $v) {
}
echo $k, $v, "\n";
$c = 0;
foreach ([[], [[]]] as $inner) {
    foreach ($inner as $x) {
        $c++;
    }
}
echo $c, "\n";
