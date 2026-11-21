<?php
class Key
{
}
$arr = ["ok" => 1];
echo "ok ", $arr["ok"], "\n";
$arr[new Key] = 2;
