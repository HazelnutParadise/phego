<?php
$n = null;
foreach ($n as &$v) {
    echo "no";
}
$s = "abc";
foreach ($s as &$v) {
    echo "no";
}
foreach ($undefined as &$v) {
    echo "no";
}
echo "[", $n, "]\n";
