--TEST--
Array copies separate ordinary elements but preserve reference aliases
--FILE--
<?php
$value = 1;
$source = ['plain' => 10, 'ref' => &$value];
$copy = $source;
$copy['plain'] = 20;
$copy['ref'] = 2;
echo $source['plain'], ':', $copy['plain'], "\n";
echo $value, ':', $source['ref'], ':', $copy['ref'], "\n";
?>
--EXPECT--
10:20
2:2:2
