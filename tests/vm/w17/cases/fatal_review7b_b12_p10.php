<?php
class A { private function __construct() { echo "A\n"; } static function make() { return new static; } }
class B extends A { function __construct() { parent::__construct(); echo "B\n"; } }
new B;
