<?php
$v = [null, true, false, 0, 1, 0.0, 1.0, "0", "", "1", [], [0], [1 => 1], ["1" => 1]];
$names = ["null", "true", "false", "0", "1", "0.0", "1.0", "'0'", "''", "'1'", "[]", "[0]", "[1=>1]", "['1'=>1]"];
$i = 0;
while ($i < 14) {
    echo $names[$i], ":";
    $j = 0;
    while ($j < 14) {
        echo $v[$i] === $v[$j] ? " T" : " .";
        $j++;
    }
    echo "\n";
    $i++;
}
$inf = 1e308 * 10;
$nan = $inf - $inf;
echo $nan === $nan ? "nan===nan\n" : "nan!==nan\n";
echo $nan == $nan ? "nan==nan\n" : "nan!=nan\n";
echo 0.0 === -0.0 ? "zero===negzero\n" : "zero!==negzero\n";
echo 1 <> 2 ? "ne\n" : "eq\n";
