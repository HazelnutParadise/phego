<?php
class A { static function rr() { return "r"; } }
echo "before\n";
echo Closure::fromCallable("rr")(), "\n";
