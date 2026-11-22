<?php
class D { public $n; function __construct($n) { $this->n = $n; } function __destruct() { echo "d", $this->n, "\n"; } }
echo "before\n";
Closure::fromCallable(new D("fc"));
