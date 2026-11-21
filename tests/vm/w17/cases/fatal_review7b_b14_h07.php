<?php
class A { public readonly ?int $i; function __construct() { $this->i[] = 1; } }
new A;
