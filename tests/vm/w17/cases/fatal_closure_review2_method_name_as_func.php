<?php
class A { function zzz() { return "method"; } static function yyy() { return "smethod"; } }
echo "before\n";
$f = "yyy";
echo $f(), "\n";
