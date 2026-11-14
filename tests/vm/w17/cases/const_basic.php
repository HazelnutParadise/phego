<?php
const GREETING = "hello";
const COUNT = 3, FLAG = true, NOTHING = null, RATIO = 2.5;
echo GREETING, " ", COUNT, " ", FLAG, " [", NOTHING, "] ", RATIO, "\n";
echo GREETING . "!", COUNT * 2, "\n";
$x = COUNT + RATIO;
echo $x, "\n";
function show()
{
    return GREETING . COUNT;
}
echo show(), "\n";
const Later = "declared below the function";
function reads_later()
{
    return Later;
}
echo reads_later(), "\n";
const lower = 1, Mixed = 2, UPPER = 3;
echo lower, Mixed, UPPER, "\n";
if (COUNT > 2) {
    echo "big\n";
}
echo COUNT === 3 ? "int" : "other", "\n";
$arr = [GREETING => COUNT, COUNT => GREETING];
echo $arr["hello"], $arr[3], "\n";
echo "{$arr[COUNT]}\n";
