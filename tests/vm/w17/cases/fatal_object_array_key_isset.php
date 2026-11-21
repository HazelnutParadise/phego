<?php
class Key
{
}
$arr = [1];
echo "isset with int key ", (isset($arr[0]) ? "yes" : "no"), "\n";
echo isset($arr[new Key]) ? "yes" : "no", "\n";
