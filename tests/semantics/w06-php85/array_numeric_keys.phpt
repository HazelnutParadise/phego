--TEST--
Integer string array keys respect canonical spelling and platform range
--FILE--
<?php
$overflowPositive = PHP_INT_SIZE === 4 ? "2147483648" : "9223372036854775808";
$overflowNegative = PHP_INT_SIZE === 4 ? "-2147483649" : "-9223372036854775809";
$keys = ["0", "-0", "01", "-01", "+1", "1.0", "1e2", "1\0", (string) PHP_INT_MAX, (string) PHP_INT_MIN, $overflowPositive, $overflowNegative];
foreach ($keys as $key) {
    $array = [$key => true];
    echo is_int(array_key_first($array)) ? "integer\n" : "string\n";
}
?>
--EXPECT--
integer
string
string
string
string
string
string
string
integer
integer
string
string
