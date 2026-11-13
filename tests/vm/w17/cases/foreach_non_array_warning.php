<?php
$n = null;
foreach ($n as $v) {
    echo "null body\n";
}
echo "after null\n";
$i = 5;
foreach ($i as $k => $v) {
    echo "int body\n";
}
echo "after int\n";
$s = "abc";
foreach ($s as $v) {
    echo "string body\n";
}
echo "after string\n";
$f = false;
foreach ($f as $v) {
    echo "false body\n";
}
echo "after false\n";
$t = true;
foreach ($t as $v) {
    echo "true body\n";
}
echo "after true\n";
$d = 1.5;
foreach ($d as $v) {
    echo "float body\n";
}
echo "after float\n";
foreach ("literal" as $v) {
    echo "literal body\n";
}
foreach (null as $v) {
    echo "null literal body\n";
}
echo "done\n";
