<?php
namespace App\Models;
const VERSION = "1.2";
const LIMIT = 10, DOUBLE = LIMIT * 2;
echo VERSION, " ", LIMIT, " ", DOUBLE, "\n";
echo \App\Models\VERSION, " ", namespace\LIMIT, " ", \app\models\DOUBLE, "\n";
echo E_ALL, " ", PHP_INT_SIZE, " ", \PHP_INT_SIZE, " ", strlen_free(), "\n";
function strlen_free()
{
    return VERSION . "-" . E_WARNING;
}
echo PHP_EOL === \PHP_EOL ? "same" : "differ", "\n";
const PHP_EOL = "<eol>";
echo PHP_EOL, " ", \PHP_EOL, namespace\PHP_EOL, "\n";
echo TRUE, " ", True, " ", FALSE, "[", NULL, "]", null === NULL ? "null" : "x", "\n";
function inner()
{
    return LIMIT + DOUBLE;
}
echo inner(), "\n";
