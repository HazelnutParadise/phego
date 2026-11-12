<?php
foreach ($nothing as $v) {
    echo "body\n";
}
echo "after\n";
foreach ($nothing2 as $k => $v) {
    echo "body\n";
}
echo isset($v) ? "v set" : "v unset", "\n";
$a = ["k" => 1];
foreach ($a["missing"] as $v) {
    echo "body\n";
}
foreach ($a["k"] as $v) {
    echo "body\n";
}
echo "done\n";
