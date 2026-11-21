<?php
class A { static function m() {} static function t() { return Closure::fromCallable("parent::m"); } }
echo "before\n";
A::t();
